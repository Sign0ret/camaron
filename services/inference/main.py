import json
import os
import platform
import random
import time
import threading
from datetime import datetime
from fractions import Fraction
from http.server import BaseHTTPRequestHandler, HTTPServer

import av
import numpy as np
from PIL import Image
import requests

ORCHESTRATOR_URL = os.getenv("ORCHESTRATOR_URL", "http://localhost:8080")
ORCHESTRATOR_API_KEY = os.getenv("ORCHESTRATOR_API_KEY", "")
POLL_INTERVAL = int(os.getenv("POLL_INTERVAL", "10"))
SAMPLE_INTERVAL = float(os.getenv("SAMPLE_INTERVAL", "1.0"))

# Trigger CI rebuild after workflow filter fix

# ── Cloudflare R2 config ───────────────────────────────
R2_BUCKET = os.getenv("R2_BUCKET_NAME")
R2_ACCESS_KEY = os.getenv("R2_ACCESS_KEY_ID")
R2_SECRET_KEY = os.getenv("R2_SECRET_ACCESS_KEY")
R2_ENDPOINT = os.getenv("R2_ENDPOINT_URL")

_r2_client = None
_turso_available = False

try:
    from turso import record_upload as _turso_record_upload, set_online as _turso_set_online
    _turso_available = True
except Exception:
    pass


def get_r2_client():
    global _r2_client
    if _r2_client is not None:
        return _r2_client
    if not all([R2_BUCKET, R2_ACCESS_KEY, R2_SECRET_KEY, R2_ENDPOINT]):
        return None
    import boto3
    _r2_client = boto3.client(
        "s3",
        endpoint_url=R2_ENDPOINT,
        aws_access_key_id=R2_ACCESS_KEY,
        aws_secret_access_key=R2_SECRET_KEY,
    )
    return _r2_client


def _report_status(camera_id: str, event: str):
    """Notify the orchestrator of camera state changes."""
    try:
        headers = {"Content-Type": "application/json"}
        if ORCHESTRATOR_API_KEY:
            headers["X-API-Key"] = ORCHESTRATOR_API_KEY
        requests.post(
            f"{ORCHESTRATOR_URL}/camera-status",
            json={"id": camera_id, "event": event},
            headers=headers,
            timeout=5,
        )
    except Exception:
        pass  # best-effort; don't crash the stream on orchestrator hiccups


def _log_to_turso(camera_id: str, filename: str, event: str):
    """Optionally write directly to Turso for redundancy."""
    if not _turso_available:
        return
    try:
        if event == "chunk_uploaded":
            _turso_record_upload(camera_id, filename)
        elif event == "connected":
            _turso_set_online(camera_id, True)
        elif event == "disconnected":
            _turso_set_online(camera_id, False)
    except Exception:
        pass


def open_input(url: str):
    """Open a media input from RTSP URL or local device."""
    if url.startswith("device://"):
        device_id = url[len("device://"):]
        system = platform.system()
        if system == "Darwin":
            attempts = [
                (f"{device_id}:none", {"framerate": "30", "video_size": "1280x720"}),
                (f"{device_id}:none", {}),
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
        return av.open(url, timeout=(5, 5))


class Mp4Chunker:
    """Buffers decoded frames in memory and flushes a 5-second MP4 to R2."""

    def __init__(self, camera_id: str, chunk_sec: float = 5.0):
        self.camera_id = camera_id
        # Stagger flush windows per camera to avoid thundering herd.
        # A 1.5-second jitter spreads encode/upload load across time.
        jitter = random.uniform(0, 1.5)
        self.chunk_sec = chunk_sec + jitter
        self._buffer: list[tuple[datetime, np.ndarray]] = []
        self._chunk_start_ts: float | None = None
        self._lock = threading.Lock()

    def add_frame(self, ts: datetime, arr: np.ndarray):
        with self._lock:
            if not self._buffer:
                self._chunk_start_ts = time.time()
            self._buffer.append((ts, arr))

            elapsed = time.time() - self._chunk_start_ts
            if elapsed >= self.chunk_sec:
                self._flush()

    def stop(self):
        with self._lock:
            self._flush()

    def _flush(self):
        if not self._buffer:
            return

        # Need at least 2 frames for a valid MP4
        if len(self._buffer) < 2:
            print(f"[{self.camera_id}] skipping chunk: only {len(self._buffer)} frame(s)")
            self._buffer = []
            self._chunk_start_ts = None
            return

        tmp_path = self._encode_mp4()
        if tmp_path:
            self._upload_mp4_to_r2(tmp_path)
            try:
                os.remove(tmp_path)
            except OSError:
                pass

        self._buffer = []
        self._chunk_start_ts = None

    def _encode_mp4(self) -> str | None:
        ts_str = self._buffer[0][0].strftime("%Y%m%d_%H%M%S")
        tmp_path = f"/tmp/chunk_{self.camera_id}_{ts_str}.mp4"

        fps = round(1.0 / SAMPLE_INTERVAL)
        width = self._buffer[0][1].shape[1]
        height = self._buffer[0][1].shape[0]

        try:
            container = av.open(tmp_path, mode="w")
            stream = container.add_stream("libx264", rate=fps)
            stream.width = width
            stream.height = height
            stream.pix_fmt = "yuv420p"

            for _ts, arr in self._buffer:
                frame = av.VideoFrame.from_ndarray(arr, format="rgb24")
                frame = frame.reformat(format="yuv420p")
                for packet in stream.encode(frame):
                    container.mux(packet)

            # flush encoder
            for packet in stream.encode():
                container.mux(packet)

            container.close()
            print(f"[{self.camera_id}] encoded {tmp_path} ({len(self._buffer)} frames @ {fps}fps)")
            return tmp_path

        except Exception as e:
            print(f"[{self.camera_id}] mp4 encode failed: {e}")
            return None

    def _upload_mp4_to_r2(self, path: str):
        s3 = get_r2_client()
        if s3 is None:
            return
        key = f"{self.camera_id}/{os.path.basename(path)}"
        max_attempts = 3
        for attempt in range(max_attempts):
            try:
                s3.upload_file(path, R2_BUCKET, key)
                print(f"[{self.camera_id}] uploaded r2://{R2_BUCKET}/{key}")
                _report_status(self.camera_id, "chunk_uploaded")
                _log_to_turso(self.camera_id, os.path.basename(path), "chunk_uploaded")
                return
            except Exception as e:
                print(f"[{self.camera_id}] r2 upload attempt {attempt + 1}/{max_attempts} failed: {e}")
                if attempt < max_attempts - 1:
                    # Exponential backoff: 2s, then 4s. Sleep yields CPU to other threads.
                    time.sleep(2 ** attempt * 2)
        print(f"[{self.camera_id}] r2 upload permanently failed after {max_attempts} attempts")


class StreamWorker(threading.Thread):
    def __init__(self, camera_id: str, url: str):
        super().__init__(daemon=True)
        self.camera_id = camera_id
        self.url = url
        self._stop_event = threading.Event()
        self._container = None
        self._lock = threading.Lock()
        self._chunker = Mp4Chunker(camera_id)

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
        self._chunker.stop()
        _report_status(self.camera_id, "disconnected")
        _log_to_turso(self.camera_id, "", "disconnected")

    def run(self):
        log_prefix = f"[{self.camera_id}]"

        while not self._stop_event.is_set():
            container = None
            try:
                container = open_input(self.url)
                with self._lock:
                    self._container = container
                stream = container.streams.video[0]
                last_sample_time = 0.0
                print(f"{log_prefix} connected to {self.url}")
                _report_status(self.camera_id, "connected")
                _log_to_turso(self.camera_id, "", "connected")

                try:
                    for packet in container.demux(stream):
                        if self._stop_event.is_set():
                            break

                        now = time.time()

                        # Skip decoding most packets when not ready to sample.
                        # We still decode keyframes to keep the decoder's reference frames valid.
                        if now - last_sample_time < SAMPLE_INTERVAL:
                            if packet.is_keyframe:
                                try:
                                    for _ in packet.decode():
                                        pass
                                except Exception:
                                    pass
                            continue

                        for frame in packet.decode():
                            if self._stop_event.is_set():
                                break

                            arr = frame.to_ndarray(format="rgb24")

                            # ═══════════════════════════════════════════════════
                            # TODO: Inference Pipeline (YOLO + ByteTrack + Turso)
                            # ───────────────────────────────────────────────────
                            annotated_arr = arr  # placeholder
                            # ═══════════════════════════════════════════════════

                            ts = datetime.utcnow()
                            self._chunker.add_frame(ts, annotated_arr)
                            last_sample_time = now
                            break

                finally:
                    try:
                        container.close()
                    except Exception:
                        pass
                    with self._lock:
                        self._container = None
                    _report_status(self.camera_id, "disconnected")
                    _log_to_turso(self.camera_id, "", "disconnected")

            except Exception as e:
                if self._stop_event.is_set():
                    break
                print(f"{log_prefix} error: {e}")
                _report_status(self.camera_id, "disconnected")
                _log_to_turso(self.camera_id, "", "disconnected")
                time.sleep(5)

        # final flush on thread exit
        self._chunker.stop()


class _HealthHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/health":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"status": "ok"}).encode())
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        pass  # silence built-in logging


def _start_health_server(port: int = 8081):
    def run():
        server = HTTPServer(("", port), _HealthHandler)
        server.serve_forever()

    t = threading.Thread(target=run, daemon=True)
    t.start()
    print(f"[health] listening on :{port}")


class WorkerPool:
    def __init__(self):
        self._workers: dict[str, StreamWorker] = {}
        self._lock = threading.Lock()

    def sync(self, configs: list[dict]):
        with self._lock:
            current_ids = set(self._workers.keys())
            target_ids = {c["id"] for c in configs}

            for cid in current_ids - target_ids:
                print(f"[pool] stopping removed camera {cid}")
                self._workers[cid].stop()
                del self._workers[cid]

            for cfg in configs:
                cid = cfg["id"]
                if cid not in self._workers:
                    print(f"[pool] starting camera {cid}")
                    w = StreamWorker(cid, cfg["url"])
                    w.start()
                    self._workers[cid] = w


def main():
    _start_health_server()
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
