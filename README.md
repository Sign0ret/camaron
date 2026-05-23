# Camaron

Video ingestion pipeline. Go orchestrator manages camera registry; Python workers pull streams, decode with PyAV, and save snapshots.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Your Machine (macOS/Linux)                                 │
│                                                             │
│  ┌─────────────────┐      RTSP      ┌─────────────────────┐ │
│  │  ffmpeg /       │ ──────────────►│  MediaMTX (RTSP)    │ │
│  │  IP Camera      │   tcp://8554   │  :8554               │ │
│  └─────────────────┘                └──────────┬────────────┘ │
│                                               │              │
│                                               │ RTSP         │
│                                               ▼              │
│  ┌─────────────────┐      HTTP      ┌─────────────────────┐ │
│  │ inference worker│ ◄─────────────►│  orchestrator (Go)  │ │
│  │ (Python + PyAV) │  /stream-configs│  :8080              │ │
│  │                 │                │  Camera registry    │ │
│  │ • Decode H.264  │                └─────────────────────┘ │
│  │ • Wall-clock    │                                         │
│  │   sampling      │                                         │
│  │ • Save JPEG     │                                         │
│  └─────────────────┘                                         │
└─────────────────────────────────────────────────────────────┘
```

**Go orchestrator** (`services/orchestrator/`)
- Thread-safe camera registry (`internal/camera_manager.go`)
- HTTP API: `POST /cameras`, `DELETE /cameras/{id}`, `GET /cameras`, `GET /stream-configs`
- No video bytes. Pure control plane.

**Python worker** (`services/inference/`)
- Polls `/stream-configs` to discover cameras
- Opens RTSP or local device via PyAV
- Decodes at stream rate; wall-clock gate skips heavy work between samples
- Saves JPEG snapshots to disk

**MediaMTX** (`services/docker-compose.yml`)
- Lightweight RTSP server for ingesting streams from remote cameras
- Receives push from `ffmpeg` on your local machine

---

## Prerequisites

- **VPS**: Ubuntu server with Docker & Docker Compose installed
- **Local machine**: macOS or Linux with `ffmpeg` and `curl`
- **Firewall**: Ports `8080` (orchestrator API) and `8554` (RTSP ingest) open

Install `ffmpeg` on macOS:
```bash
brew install ffmpeg
```

---

## Deploy to VPS

### 1. First-time VPS setup

Copy the initialization script and run it:

```bash
scp scripts/vps-init.sh root@YOUR_VPS_IP:/root/vps-init.sh
ssh root@YOUR_VPS_IP "bash /root/vps-init.sh"
```

This installs Docker, authenticates with GHCR, writes the production compose file, and starts the stack.

### 2. Update the stack (after code changes)

Push to `main` and GitHub Actions will automatically build images and deploy:

```bash
git push origin main
```

Or manually from your machine:

```bash
scp services/docker-compose.prod.yml root@YOUR_VPS_IP:/opt/camaron/
scp scripts/deploy.sh root@YOUR_VPS_IP:/opt/camaron/
ssh root@YOUR_VPS_IP "cd /opt/camaron && bash deploy.sh"
```

### 3. Verify services are running

```bash
ssh root@YOUR_VPS_IP "docker ps"
ssh root@YOUR_VPS_IP "curl -s http://localhost:8080/health"
```

Expected: `{"status":"ok"}`

---

## Stream Your Camera

### Option A: macOS Webcam → VPS

Run this on your Mac. It pushes your built-in camera to the VPS via RTSP:

```bash
ffmpeg -re -f avfoundation -framerate 30 -video_size 1280x720 -pix_fmt yuv420p -i "0" \
  -c:v libx264 -preset ultrafast -tune zerolatency -pix_fmt yuv420p \
  -rtsp_transport tcp \
  -f rtsp rtsp://YOUR_VPS_IP:8554/macbook
```

**Key flags explained:**
- `-re`: read input at native frame rate (prevents flooding)
- `-pix_fmt yuv420p`: compatible pixel format for H.264
- `-rtsp_transport tcp`: reliable transport through NAT/firewalls
- `-i "0"`: avfoundation device index 0 (usually FaceTime HD Camera)

### Option B: IP Camera / RTSP Camera

If you have an existing RTSP camera accessible from the VPS, skip `ffmpeg` and use the camera's direct URL (e.g., `rtsp://192.168.1.100/stream`).

---

## Register Your Camera

Once the stream is being pushed (or ready), register it with the orchestrator:

```bash
curl -X POST http://YOUR_VPS_IP:8080/cameras \
  -H "Content-Type: application/json" \
  -d '{"id":"macbook","url":"rtsp://mediamtx:8554/macbook"}'
```

For an IP camera directly reachable by the VPS:

```bash
curl -X POST http://YOUR_VPS_IP:8080/cameras \
  -H "Content-Type: application/json" \
  -d '{"id":"garage","url":"rtsp://192.168.1.100/stream"}'
```

Verify registration:

```bash
curl http://YOUR_VPS_IP:8080/cameras
```

---

## Verify Everything Works

### 1. Camera registry

```bash
ssh root@YOUR_VPS_IP "curl -s http://localhost:8080/cameras"
```

### 2. Inference worker logs

```bash
ssh root@YOUR_VPS_IP "docker logs -f camaron-inference-1"
```

Look for: `Saving snapshot for macbook ...`

If you see `404 Not Found`, `ffmpeg` is not successfully publishing to MediaMTX. Check your `ffmpeg` command and MediaMTX logs.

### 3. MediaMTX logs

```bash
ssh root@YOUR_VPS_IP "docker logs -f camaron-mediamtx-1"
```

Look for your camera ID (`macbook`) in the publish/connection logs.

### 4. Check snapshots on disk

```bash
ssh root@YOUR_VPS_IP "ls -lah /data/camaron/snapshots/macbook/ | tail -10"
```

Expected: `.jpg` files with recent timestamps.

### 5. One-liner full health check

```bash
ssh root@YOUR_VPS_IP "
  echo '--- Cameras ---' &&
  curl -s http://localhost:8080/cameras &&
  echo &&
  echo '--- Inference (last 5 lines) ---' &&
  docker logs --tail 5 camaron-inference-1 &&
  echo &&
  echo '--- Latest snapshots ---' &&
  ls -lt /data/camaron/snapshots/macbook/ 2>/dev/null | head -5 &&
  echo &&
  echo '--- Snapshots in last 60s ---' &&
  find /data/camaron/snapshots/macbook/ -type f -mmin -1 | wc -l
"
```

---

## View Snapshots Locally

### Sync all snapshots to your Mac

```bash
mkdir -p ~/camaron-snapshots
rsync -avz root@YOUR_VPS_IP:/data/camaron/snapshots/ ~/camaron-snapshots/
open ~/camaron-snapshots/macbook
```

Re-run the same `rsync` later — it will only copy new files.

### Quick preview (single latest image)

```bash
ssh root@YOUR_VPS_IP "ls -t /data/camaron/snapshots/macbook/*.jpg | head -1" | \
  xargs -I {} scp root@YOUR_VPS_IP:{} /tmp/latest.jpg && open /tmp/latest.jpg
```

---

## API Reference

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | `GET` | Health check. Returns `{"status":"ok"}` |
| `/cameras` | `GET` | List all registered cameras |
| `/cameras` | `POST` | Register a camera. Body: `{"id":"x","url":"rtsp://..."}` |
| `/cameras/{id}` | `GET` | Get a single camera config |
| `/cameras/{id}` | `DELETE` | Remove a camera |
| `/stream-configs` | `GET` | Alias for `GET /cameras` (used by inference worker) |

---

## Local Development

If you want to develop on your laptop without a VPS:

```bash
# 1. Start orchestrator + RTSP relay locally
cd services && docker compose up -d orchestrator mediamtx

# 2. Register your local camera
curl -X POST http://localhost:8080/cameras \
  -H "Content-Type: application/json" \
  -d '{"id":"macbook","url":"device://0"}'

# 3. Run inference worker on host (macOS Docker cannot access cameras)
cd services/inference && ./run-local.sh

# 4. Check output
ls services/inference/data/snapshots/macbook/
```

---

## Troubleshooting

| Symptom | Likely Cause | Fix |
|---------|--------------|-----|
| `404 Not Found` in inference logs | ffmpeg not publishing to MediaMTX | Restart ffmpeg with `-re -rtsp_transport tcp` |
| `broken pipe` in ffmpeg | Flooding server without `-re` | Add `-re` flag |
| `Connection refused` on port 8554 | Firewall blocking RTSP | Open port 8554 on VPS and cloud provider |
| No snapshots appearing | Inference hasn't polled yet | Wait 10s (default `POLL_INTERVAL`) |
| Stale snapshots | Stream dropped | Restart ffmpeg and check MediaMTX logs |
| Disk full | Snapshots accumulate forever | `find /data/camaron/snapshots/ -mtime +7 -delete` |

---

## Key Files

| File | Role |
|------|------|
| `services/orchestrator/main.go` | HTTP API server |
| `services/orchestrator/internal/camera_manager.go` | Thread-safe registry |
| `services/inference/main.py` | PyAV decoder + worker pool |
| `services/inference/run-local.sh` | Host runner for macOS camera access |
| `services/docker-compose.yml` | Local development stack |
| `services/docker-compose.prod.yml` | Production VPS stack |
| `scripts/deploy.sh` | Zero-downtime VPS deploy script |
| `scripts/vps-init.sh` | First-time VPS setup script |

---

## Future Work

- Replace JPEG saving with ONNX inference
- WebSocket/SSE push for camera config changes (replaces polling)
- Web dashboard (`apps/web`)
- Persistent camera registry (currently in-memory)
