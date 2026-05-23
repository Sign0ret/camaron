# Camaron

Stream any camera to a VPS. Frames are buffered in memory and flushed as 5-second MP4 chunks to Cloudflare R2 every 5 seconds.

## What you need

- A VPS (Ubuntu + Docker)
- A Mac with `ffmpeg` installed: `brew install ffmpeg`
- A Cloudflare R2 bucket + API token

## Setup

### 1. VPS first-time init

```bash
scp scripts/vps-init.sh root@YOUR_VPS_IP:/root/vps-init.sh
ssh root@YOUR_VPS_IP "bash /root/vps-init.sh"
```

### 2. Add R2 credentials to GitHub

Go to `Settings → Secrets and variables → Actions` in your repo and add:

- `R2_BUCKET_NAME`
- `R2_ACCESS_KEY_ID`
- `R2_SECRET_ACCESS_KEY`
- `R2_ENDPOINT_URL` (format: `https://<account_id>.r2.cloudflarestorage.com`)

### 3. Deploy

```bash
git push origin main
```

GitHub Actions builds images and deploys to the VPS automatically. The `.env` file is written from secrets on every deploy.

Verify the stack is up:

```bash
ssh root@YOUR_VPS_IP "curl -s http://localhost:8080/health"
# → {"status":"ok"}
```

---

## Add a camera

Two steps: stream to the VPS, then register it.

### Step A — Stream

Run this on your Mac. Replace `CAMERA_ID` with any name you want (e.g., `macbook`, `livingroom`, `garage`):

```bash
ffmpeg -re -f avfoundation -framerate 30 -video_size 1280x720 -pix_fmt yuv420p -i "0" \
  -c:v libx264 -preset ultrafast -tune zerolatency -pix_fmt yuv420p \
  -rtsp_transport tcp \
  -f rtsp rtsp://YOUR_VPS_IP:8554/CAMERA_ID
```

Keep this terminal open. `ffmpeg` is now pushing your camera to the VPS MediaMTX server.

### Step B — Register

Tell the orchestrator about this camera so the inference worker starts pulling it:

```bash
curl -X POST http://YOUR_VPS_IP:8080/cameras \
  -H "Content-Type: application/json" \
  -d '{"id":"CAMERA_ID","url":"rtsp://mediamtx:8554/CAMERA_ID"}'
```

Replace `CAMERA_ID` with the same name you used in the ffmpeg command.

**Verify registration:**

```bash
curl http://YOUR_VPS_IP:8080/cameras
# → [{"id":"CAMERA_ID","url":"rtsp://mediamtx:8554/CAMERA_ID"}]
```

### Step C — Confirm in logs

Watch the inference worker pick up the new camera in real time:

```bash
ssh root@YOUR_VPS_IP "docker logs -f camaron-inference-1"
```

You should see within ~10 seconds:

```
[pool] starting camera CAMERA_ID
[CAMERA_ID] connected to rtsp://mediamtx:8554/CAMERA_ID
[CAMERA_ID] encoded /tmp/chunk_CAMERA_ID_20240115_143000.mp4 (5 frames @ 1fps)
[CAMERA_ID] uploaded r2://BUCKET/CAMERA_ID/chunk_CAMERA_ID_20240115_143000.mp4
```

A new `.mp4` file appears in R2 every ~5 seconds.

---

## Delete a camera

Stop the ffmpeg stream on your Mac, then:

```bash
curl -X DELETE http://YOUR_VPS_IP:8080/cameras/CAMERA_ID
```

The inference worker will stop automatically on the next poll.

---

## View recordings

Open your Cloudflare R2 dashboard. Each camera has its own folder (`CAMERA_ID/`) containing `.mp4` files named by timestamp.

---

## Architecture

```
Mac ffmpeg → VPS MediaMTX (8554) → inference worker → R2 MP4 chunks
                                ↘ orchestrator (8080) ← API
```

- **orchestrator** (Go, port 8080): camera registry REST API
- **mediamtx** (port 8554): RTSP server that receives the ffmpeg push
- **inference** (Python + PyAV): buffers decoded frames in memory, flushes a 5-second MP4 chunk every 5 seconds, uploads to R2

---

## API

| Endpoint | Method | Body | Description |
|---|---|---|---|
| `/health` | `GET` | — | Health check |
| `/cameras` | `GET` | — | List all cameras |
| `/cameras` | `POST` | `{"id":"x","url":"rtsp://..."}` | Register a camera |
| `/cameras/{id}` | `GET` | — | Get one camera |
| `/cameras/{id}` | `DELETE` | — | Remove a camera |

---

## Troubleshooting

| Problem | Fix |
|---|---|
| `404 Not Found` in inference logs | ffmpeg is not connected. Check ffmpeg is running and the `CAMERA_ID` matches exactly between ffmpeg path and POST body. |
| `broken pipe` in ffmpeg | Add `-re` flag so ffmpeg does not flood the server. |
| No `starting camera` in logs | Wait 10s for poll interval. Verify with `curl /cameras`. |
| `r2 upload failed` | Check `.env` on VPS (`cat /opt/camaron/.env`) and GitHub Secrets are correct. |
| Port 8554 refused | Open port 8554 in VPS firewall (`ufw allow 8554`) and cloud provider security group. |

---

## Local Development

Run the stack locally without a VPS:

```bash
cd services && docker compose up -d orchestrator mediamtx
curl -X POST http://localhost:8080/cameras \
  -H "Content-Type: application/json" \
  -d '{"id":"macbook","url":"device://0"}'
cd services/inference && ./run-local.sh
```

See `services/docker-compose.yml` for the local stack and `services/docker-compose.prod.yml` for the VPS stack.
