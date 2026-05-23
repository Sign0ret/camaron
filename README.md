# Camaron

Stream any camera to a VPS, save snapshots, and view them locally.

## Quickstart

You need a VPS (Ubuntu + Docker), a Mac with `ffmpeg` installed, and optionally a Cloudflare R2 bucket if you want off-VPS snapshot storage.

### 1. Start the VPS stack

```bash
scp scripts/vps-init.sh root@YOUR_VPS_IP:/root/vps-init.sh
ssh root@YOUR_VPS_IP "bash /root/vps-init.sh"
```

The `.env` file on the VPS is managed automatically by GitHub Actions. Add R2 credentials as **GitHub Repository Secrets** (`Settings → Secrets and variables → Actions`):

- `R2_BUCKET_NAME`
- `R2_ACCESS_KEY_ID`
- `R2_SECRET_ACCESS_KEY`
- `R2_ENDPOINT_URL` (format: `https://<account_id>.r2.cloudflarestorage.com`)

The next deploy will write these to `/opt/camaron/.env` on the VPS automatically.

Verify:
```bash
ssh root@YOUR_VPS_IP "curl -s http://localhost:8080/health"
# → {"status":"ok"}
```

### 2. Stream your Mac camera to the VPS

```bash
ffmpeg -re -f avfoundation -framerate 30 -video_size 1280x720 -pix_fmt yuv420p -i "0" \
  -c:v libx264 -preset ultrafast -tune zerolatency -pix_fmt yuv420p \
  -rtsp_transport tcp \
  -f rtsp rtsp://YOUR_VPS_IP:8554/macbook
```

### 3. Register the camera

```bash
curl -X POST http://YOUR_VPS_IP:8080/cameras \
  -H "Content-Type: application/json" \
  -d '{"id":"macbook","url":"rtsp://mediamtx:8554/macbook"}'
```

### 4. Verify it is working

```bash
ssh root@YOUR_VPS_IP "
  echo '--- Cameras ---' &&
  curl -s http://localhost:8080/cameras &&
  echo &&
  echo '--- Latest snapshots ---' &&
  ls -lt /data/camaron/snapshots/macbook/ 2>/dev/null | head -5 &&
  echo &&
  echo '--- Snapshots in last 60s ---' &&
  find /data/camaron/snapshots/macbook/ -type f -mmin -1 | wc -l
"
```

### 5. View snapshots

If you configured R2, open the R2 dashboard or use any S3-compatible tool.  
If snapshots are only on the VPS disk, copy them to your Mac:

```bash
mkdir -p ~/camaron-snapshots
rsync -avz root@YOUR_VPS_IP:/data/camaron/snapshots/ ~/camaron-snapshots/
open ~/camaron-snapshots/macbook
```

---

## API

| Endpoint | Method | Body | Description |
|---|---|---|---|
| `/health` | `GET` | — | Health check |
| `/cameras` | `GET` | — | List cameras |
| `/cameras` | `POST` | `{"id":"x","url":"rtsp://..."}` | Register camera |
| `/cameras/{id}` | `DELETE` | — | Remove camera |

---

## Architecture

```
Mac ffmpeg → VPS MediaMTX (8554) → inference worker → snapshots
                                ↘ orchestrator (8080) ← API
```

- **orchestrator** (Go): camera registry REST API
- **inference** (Python + PyAV): pulls RTSP, saves JPEG every 1s
- **mediamtx**: RTSP server that receives the ffmpeg push

---

## Troubleshooting

| Problem | Fix |
|---|---|
| Inference logs show `404 Not Found` | ffmpeg is not connected. Restart ffmpeg and check MediaMTX logs. |
| ffmpeg says `broken pipe` | Add `-re` so it does not flood the server. |
| No snapshots after registering | Wait 10s for inference to poll, then check logs. |
| Port 8554 refused | Open port 8554 in your VPS firewall and cloud provider. |

---

## Local Development

Run the stack locally without a VPS:

```bash
cd services && docker compose up -d orchestrator mediamtx
curl -X POST http://localhost:8080/cameras \
  -H "Content-Type: application/json" \
  -d '{"id":"macbook","url":"device://0"}'
cd services/inference && ./run-local.sh
ls services/inference/data/snapshots/macbook/
```

See `services/docker-compose.yml` for the local stack and `services/docker-compose.prod.yml` for the VPS stack.
