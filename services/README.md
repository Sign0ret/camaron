# Services

Local development stack for the orchestrator.

## Prerequisites

- Docker
- Docker Compose

---

## Local Development

### Start

```bash
cd services && docker compose up --build
```

This launches three containers:

| Container | Port | Role |
|-----------|------|------|
| `orchestrator` | 8080 | Go binary — RTSP ingestion + HTTP API |
| `mediamtx` | 8554 | RTSP server |
| `ffmpeg` | — | Generates a synthetic 640x480@30fps test pattern, publishes to mediamtx |

The orchestrator auto-connects to `rtsp://mediamtx:8554/test` as camera `test`.

### Test

```bash
# Health
curl http://localhost:8080/health

# Ping
curl http://localhost:8080/ping

# List cameras
curl http://localhost:8080/cameras

# Camera status (frames_in, frames_throttled, frames_dropped)
curl http://localhost:8080/camera/test/status | python3 -m json.tool
```

Expected output for `/camera/test/status`:

```json
{
    "id": "test",
    "url": "rtsp://mediamtx:8554/test",
    "running": true,
    "frames_in": 1373,
    "frames_throttled": 1349,
    "frames_dropped": 0
}
```

### Stop

```bash
cd services && docker compose down
```

### Troubleshooting

If `frames_in` stays at `0` after startup, the ffmpeg container may not have finished publishing yet. Restart the orchestrator:

```bash
docker compose restart orchestrator
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
