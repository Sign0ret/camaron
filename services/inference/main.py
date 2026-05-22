import os
import platform
import time
import threading
import requests
import av
import numpy as np
from PIL import Image
from datetime import datetime

ORCHESTRATOR_URL = os.getenv("ORCHESTRATOR_URL", "http://localhost:8080")
OUTPUT_DIR = os.getenv("OUTPUT_DIR", "./data/snapshots")
POLL_INTERVAL = int(os.getenv("POLL_INTERVAL", "10"))
SAMPLE_INTERVAL = float(os.getenv("SAMPLE_INTERVAL", "1.0"))


def open_input(url: str):
    """Open a media input from RTSP URL or local device."""
    if url.startswith("device://"):
        device_id = url[len("device://"):]
        system = platform.system()
        if system == "Darwin":
            # macOS AVFoundation can be finicky about framerate negotiation.
            # We try several strategies in order of preference.
            attempts = [
                # 1. Explicitly request 30fps + 1280x720, no audio
                (f"{device_id}:none", {"framerate": "30", "video_size": "1280x720"}),
                # 2. Same format, let the driver auto-negotiate
                (f"{device_id}:none", {}),
                # 3. Omit ":none" — some FFmpeg builds prefer this
                (device_id, {}),
            ]
            for device_str, options in attempts:
                try:
                    return av.open(device_str, format="avfoundation", options=options)
                except av.error.Error as e:
                    print(f"[open] attempt failed ({device_str}, {options}): {e}")
                    continue
            raise RuntimeError(
                f"Could not open macOS camera device {device_id}. "
                "Ensure your terminal has camera permission: "
                "System Settings → Privacy & Security → Camera → Terminal (or iTerm)"
            )
        elif system == "Linux":
            return av.open(f"/dev/video{device_id}", format="v4l2")
        else:
            raise RuntimeError(f"Unsupported platform for device input: {system}")
    else:
        # Assume RTSP URL
        return av.open(url, timeout=(5, 5))


class StreamWorker(threading.Thread):
    def __init__(self, camera_id: str, url: str):
        super().__init__(daemon=True)
        self.camera_id = camera_id
        self.url = url
        self._stop_event = threading.Event()
        self._container = None
        self._lock = threading.Lock()

    def stop(self):
        """Signal the worker to stop and force-unblock any I/O."""
        self._stop_event.set()
        with self._lock:
            container = self._container
        if container:
            try:
                container.close()
            except Exception:
                pass

    def run(self):
        log_prefix = f"[{self.camera_id}]"
        out_path = os.path.join(OUTPUT_DIR, self.camera_id)
        os.makedirs(out_path, exist_ok=True)

        while not self._stop_event.is_set():
            container = None
            try:
                container = open_input(self.url)
                with self._lock:
                    self._container = container
                stream = container.streams.video[0]
                last_sample_time = 0.0
                print(f"{log_prefix} connected to {self.url}")

                try:
                    for packet in container.demux(stream):
                        if self._stop_event.is_set():
                            break
                        for frame in packet.decode():
                            if self._stop_event.is_set():
                                break

                            # --- backpressure gate ---
                            now = time.time()
                            if now - last_sample_time < SAMPLE_INTERVAL:
                                continue
                            last_sample_time = now
                            # -------------------------

                            arr = frame.to_ndarray(format="rgb24")
                            img = Image.fromarray(arr)
                            ts = datetime.utcnow().strftime("%Y%m%d_%H%M%S_%f")
                            filename = os.path.join(out_path, f"frame_{ts}.jpg")
                            img.save(filename, quality=85)
                            print(f"{log_prefix} saved {filename}")
                finally:
                    # --- explicit container cleanup ---
                    try:
                        container.close()
                    except Exception:
                        pass
                    with self._lock:
                        self._container = None
            except Exception as e:
                if self._stop_event.is_set():
                    break
                print(f"{log_prefix} error: {e}")
                time.sleep(5)


class WorkerPool:
    def __init__(self):
        self._workers: dict[str, StreamWorker] = {}
        self._lock = threading.Lock()

    def sync(self, configs: list[dict]):
        with self._lock:
            current_ids = set(self._workers.keys())
            target_ids = {c["id"] for c in configs}

            # Stop removed cameras
            for cid in current_ids - target_ids:
                print(f"[pool] stopping removed camera {cid}")
                self._workers[cid].stop()
                del self._workers[cid]

            # Start new cameras
            for cfg in configs:
                cid = cfg["id"]
                if cid not in self._workers:
                    print(f"[pool] starting camera {cid}")
                    w = StreamWorker(cid, cfg["url"])
                    w.start()
                    self._workers[cid] = w


def main():
    os.makedirs(OUTPUT_DIR, exist_ok=True)
    pool = WorkerPool()

    while True:
        try:
            resp = requests.get(f"{ORCHESTRATOR_URL}/stream-configs", timeout=5)
            resp.raise_for_status()
            configs = resp.json()
            pool.sync(configs)
        except Exception as e:
            print(f"[main] failed to poll orchestrator: {e}")
        time.sleep(POLL_INTERVAL)


if __name__ == "__main__":
    main()
