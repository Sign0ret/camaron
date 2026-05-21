# Services

Local development stack for the orchestrator.

## Prerequisites

- [Colima](https://github.com/abiosoft/colima) (Docker runtime)
- Docker + Docker Compose
- Go ≥ 1.25
- ffmpeg (for real camera streaming)

```bash
brew install colima docker docker-compose go ffmpeg
```

---

## Quickstart (stream sintético)

### Start

Three terminals needed.

**Terminal 1 — Docker infrastructure:**

```bash
colima start
cd services
docker-compose up mediamtx minio ffmpeg -d
sleep 3
```

| Container | Port | Role |
|-----------|------|------|
| `mediamtx` | 8554 | RTSP server |
| `minio` | 9000, 9001 | S3-compatible storage (local R2 alternative) |
| `ffmpeg` | — | Generates synthetic 640x480@30fps test pattern → mediamtx |

**Terminal 2 — Orchestrator:**

```bash
cd services/orchestrator
go build -o orchestrator .
CAMERA_URLS=test=rtsp://mediamtx:8554/test \
  S3_ENDPOINT=http://localhost:9000 \
  S3_BUCKET=camaron \
  S3_ACCESS_KEY_ID=minioadmin \
  S3_SECRET_ACCESS_KEY=minioadmin \
  S3_REGION=us-east-1 \
  S3_USE_PATH_STYLE=true \
  RECORDING_DIR=/tmp/camaron/recordings \
  ./orchestrator
```

**Terminal 3 — Verify:**

```bash
curl http://localhost:8080/health          # {"status":"ok"}
curl http://localhost:8080/cameras         # ["test"]
curl http://localhost:8080/camera/test/status
ls -la /tmp/camaron/recordings/test/

# MinIO console
open http://localhost:9001  # minioadmin / minioadmin → bucket camaron
```

### Stop

```bash
# Ctrl+C in orchestrator terminal
# Ctrl+C in ffmpeg terminal
cd services && docker-compose down
colima stop
```

---

## Quickstart (cámara real de Mac)

### Start

**Terminal 1 — Docker infrastructure:**

```bash
colima start
cd services
docker-compose up mediamtx minio -d
sleep 3
```

**Terminal 2 — Cámara Mac → RTSP:**

```bash
# List devices if needed
ffmpeg -f avfoundation -list_devices true -i ""

# Stream camera (reconnects on failure)
while true; do
  ffmpeg -f avfoundation -framerate 30 -video_size 1280x720 -i "0" \
    -c:v libx264 -preset ultrafast -tune zerolatency \
    -pix_fmt yuv420p -g 30 -keyint_min 30 \
    -flags +global_header -f rtsp -rtsp_transport tcp \
    rtsp://localhost:8554/mac
  sleep 2
done
```

**Terminal 3 — Orchestrator:**

```bash
cd services/orchestrator
go build -o orchestrator .
CAMERA_URLS=mac=rtsp://mediamtx:8554/mac \
  S3_ENDPOINT=http://localhost:9000 \
  S3_BUCKET=camaron \
  S3_ACCESS_KEY_ID=minioadmin \
  S3_SECRET_ACCESS_KEY=minioadmin \
  S3_REGION=us-east-1 \
  S3_USE_PATH_STYLE=true \
  RECORDING_DIR=/tmp/camaron/recordings \
  ./orchestrator
```

**Terminal 4 — Verify:**

```bash
curl http://localhost:8080/camera/mac/status
ls -la /tmp/camaron/recordings/mac/
ffplay /tmp/camaron/recordings/mac/*.mp4
open http://localhost:9001  # minioadmin / minioadmin → bucket camaron → mac/
```

### Stop

```bash
# Ctrl+C in each terminal (orchestrator, ffmpeg)
cd services && docker-compose down
colima stop
```

---

## Architecture

```
Camera RTSP ─► orchestrator ─► MP4 segments (5s = 25 frames @ 5fps) ─► disk ─► MinIO/R2
                  │                           │                            │
            h264.Decoder              joy4 muxer                    S3 + MD5 verify
         (FU-A, STAP-A, SPS/PPS)   (atomic tmp→rename)          (retry ×3, ETag check)
```

### Endpoints

| Method | Path | Response |
|--------|------|----------|
| GET | `/health` | `{"status":"ok"}` |
| GET | `/ping` | `{"pong":"verified"}` |
| GET | `/cameras` | `["cam1","cam2"]` |
| GET | `/camera/{id}/status` | `{"id":"...","running":true,"frames_in":501,...}` |

### Env vars

| Variable | Default | Description |
|----------|---------|-------------|
| `CAMERA_URLS` | — | Comma-separated `id=rtsp_url` pairs |
| `CAMERAS_CONFIG` | — | Path to JSON file with camera list |
| `S3_ENDPOINT` | `http://minio:9000` | S3 endpoint (MinIO or R2) |
| `S3_BUCKET` | `camaron` | Bucket name |
| `S3_ACCESS_KEY_ID` | `minioadmin` | S3 access key |
| `S3_SECRET_ACCESS_KEY` | `minioadmin` | S3 secret key |
| `S3_REGION` | `us-east-1` | S3 region (`auto` for R2) |
| `S3_USE_PATH_STYLE` | `true` | Path-style for MinIO, `false` for R2 |
| `RECORDING_DIR` | `/tmp/camaron/recordings` | Local segment directory |
| `SEGMENT_FRAMES` | `25` | Frames per MP4 segment |
| `FPS_LIMIT` | `5` | Recording FPS |
| `UPLOAD_WORKERS` | `10` | Concurrent S3 upload workers |

### Multiple cameras (production)

Use `CAMERAS_CONFIG` pointing to a JSON file:

```json
{
  "cameras": [
    {"id": "cam1", "url": "rtsp://192.168.1.100:554/stream1"},
    {"id": "cam2", "url": "rtsp://192.168.1.101:554/stream1"}
  ]
}
```

```bash
CAMERAS_CONFIG=/etc/camaron/cameras.json \
  S3_ENDPOINT=https://<account>.r2.cloudflarestorage.com \
  S3_USE_PATH_STYLE=false \
  ...
```

---

## Staging VPS (178.156.164.154)

The VPS only runs the orchestrator — no cameras, no RTSP source by default.

### Working endpoints

```bash
# Health (always responds)
curl http://178.156.164.154:8080/health
# → {"status":"ok"}

# Ping
curl http://178.156.164.154:8080/ping
# → {"pong":"verified"}

# Camera list (empty — no cameras configured)
curl http://178.156.164.154:8080/cameras
# → []
```

### Expected failures (no cameras nor RTSP source on VPS)

```bash
# Non-existent camera
curl http://178.156.164.154:8080/camera/test/status
# → {"error":"camera not found"}

# No frames being ingested (check logs)
ssh root@178.156.164.154 "docker compose -f /opt/camaron/docker-compose.yml logs orchestrator 2>&1 | tail -20"
# Output will NOT contain "[test] rtsp: streaming started" — no cameras active
```

---

## CI/CD

Push to `main` with changes under `services/**` triggers:

1. `go build + test`
2. `js lint + build + typecheck`
3. `docker build + push to ghcr.io`
4. `deploy to VPS` (SSH + health validation + swap)

Deploy script: `scripts/deploy.sh`. Path filter on `services/**` in `.github/workflows/ci.yml`.
