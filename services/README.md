# Services

Local development stack for the orchestrator + inference worker.

## Prerequisites

- Docker & Docker Compose (for Go orchestrator + RTSP relay)
- Python 3.11+ (for running inference worker on host — macOS Docker cannot access host cameras)

---

## Local Development with Laptop Camera

### 1. Start the orchestrator and RTSP relay

```bash
cd services && docker compose up --build -d orchestrator mediamtx
```

This launches:

| Container      | Port  | Role                                            |
|----------------|-------|-------------------------------------------------|
| `orchestrator` | 8080  | Go binary — camera registry & control-plane API |
| `mediamtx`     | 8554  | RTSP server (optional, for future real cameras) |

### 2. Register your laptop camera

```bash
curl -X POST http://localhost:8080/cameras \
  -H "Content-Type: application/json" \
  -d '{"id":"macbook","url":"device://0"}'
```

`device://0` tells the Python worker to open the first video capture device using the native OS API (AVFoundation on macOS, V4L2 on Linux).

### 3. Run the inference worker on your host

Docker Desktop cannot access macOS camera hardware, so the worker must run natively:

```bash
cd services/inference
./run-local.sh
```

This will:
- Create a Python virtual environment (`services/inference/.venv`)
- Install PyAV, Pillow, and requests
- Connect to the orchestrator at `http://localhost:8080`
- Open your laptop camera
- Save one JPEG snapshot every **1 second** (wall-clock time)

### 4. Check the output

```bash
ls services/inference/data/snapshots/macbook/
```

You should see files like `frame_20240115_120000_000000.jpg` every second.

### 5. Verify the control plane

```bash
# List registered cameras
curl http://localhost:8080/cameras

# Get one camera config
curl http://localhost:8080/cameras/macbook

# Health check
curl http://localhost:8080/health
```

### 6. Stop

```bash
# Stop the host inference worker (Ctrl-C in its terminal)

# Stop Docker services
cd services && docker compose down
```

---

## Using an RTSP Camera Instead

If you have a real IP camera or want to use the synthetic test stream:

```bash
# 1. Start everything including the fake ffmpeg stream
cd services && docker compose up --build -d

# 2. Register the RTSP stream
curl -X POST http://localhost:8080/cameras \
  -H "Content-Type: application/json" \
  -d '{"id":"test","url":"rtsp://mediamtx:8554/test"}'

# 3. The inference container will auto-connect and save snapshots
#    (no host Python needed for pure RTSP streams)
```

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Your Laptop (macOS)                                        │
│                                                             │
│  ┌─────────────────┐      HTTP      ┌─────────────────────┐ │
│  │ inference worker│ ◄─────────────►│  orchestrator (Go)  │ │
│  │ (host Python)   │  /stream-configs│  :8080              │ │
│  │                 │                │  Camera registry    │ │
│  │ • PyAV decode   │                └─────────────────────┘ │
│  │ • Wall-clock    │                                         │
│  │   sampling      │                                         │
│  │ • Save JPEG     │                                         │
│  └────────┬────────┘                                         │
│           │ avfoundation                                     │
│           ▼                                                  │
│  ┌─────────────────┐                                         │
│  │  FaceTime HD    │                                         │
│  │  Camera (0)     │                                         │
│  └─────────────────┘                                         │
└─────────────────────────────────────────────────────────────┘
```

---

## Sampling Behavior

The inference worker uses **wall-clock time** (`time.time()`) to decide when to process a frame:

```python
now = time.time()
if now - last_sample_time >= SAMPLE_INTERVAL:
    # process frame
    last_sample_time = now
```

This guarantees a strict interval regardless of the camera's actual frame rate or network jitter. The default `SAMPLE_INTERVAL` is **1.0 second**.

---

## CI/CD

Push to `main` with changes under `services/**` triggers:

1. `go build + test`
2. `js lint + build + typecheck`
3. `docker build + push to ghcr.io`
4. `deploy to VPS` (SSH + health validation + swap)

Deploy script: `scripts/deploy.sh`. Path filter on `services/**` in `.github/workflows/ci.yml`.
