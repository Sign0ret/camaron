# Camaron

Video ingestion pipeline. Go orchestrator manages camera registry; Python workers pull streams, decode with PyAV, and run inference.

## Architecture

```
┌─────────────────┐     HTTP      ┌──────────────────┐
│   orchestrator  │ ◄────────────►│  inference worker │
│   (Go)          │  /stream-configs│  (Python + PyAV)  │
│  :8080          │                │                   │
│  Camera registry│                │  • Decode H.264   │
│  REST API       │                │  • Wall-clock     │
└─────────────────┘                │    sampling       │
                                   │  • ONNX inference │
                                   │    (future)       │
                                   └────────┬──────────┘
                                            │
                                   ┌────────▼──────────┐
                                   │  Camera / RTSP    │
                                   └───────────────────┘
```

**Go orchestrator** (`services/orchestrator/`)
- Thread-safe camera registry (`internal/camera_manager.go`)
- HTTP API: `POST /cameras`, `DELETE /cameras/{id}`, `GET /stream-configs`
- No video bytes. Pure control plane.

**Python worker** (`services/inference/`)
- Polls `/stream-configs` to discover cameras
- Opens RTSP or local device (`device://0`) via PyAV
- Decodes at stream rate; wall-clock gate skips heavy work between samples
- Stops cleanly via `container.close()` from outside the demux loop

## Run Locally

```bash
# 1. Orchestrator + RTSP relay
cd services && docker compose up -d orchestrator mediamtx

# 2. Register your camera
curl -X POST http://localhost:8080/cameras \
  -H "Content-Type: application/json" \
  -d '{"id":"macbook","url":"device://0"}'

# 3. Run inference worker on host (macOS Docker cannot access cameras)
cd services/inference && ./run-local.sh

# 4. Check output
ls services/inference/data/snapshots/macbook/
```

## Key Files

| File | Role |
|------|------|
| `services/orchestrator/main.go` | HTTP API server |
| `services/orchestrator/internal/camera_manager.go` | Thread-safe registry |
| `services/inference/main.py` | PyAV decoder + worker pool |
| `services/inference/run-local.sh` | Host runner for macOS camera access |
| `services/docker-compose.yml` | Local stack |

## Future Work

- Replace JPEG saving with ONNX inference
- WebSocket/SSE push for camera config changes (replaces polling)
- Web dashboard (`apps/web`)
- Persistent camera registry (currently in-memory)
