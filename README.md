# Camaron

Stream any camera to a VPS. Frames are buffered in memory and flushed as 5-second MP4 chunks to Cloudflare R2 every 5 seconds.

## What you need

- A VPS (Ubuntu + Docker)
- A Turso database account ([turso.tech](https://turso.tech))
- A Cloudflare R2 bucket + API token
- A Mac with `ffmpeg` installed: `brew install ffmpeg`
- Vercel account for the admin dashboard

## Architecture

```
Mac ffmpeg → VPS MediaMTX (8554) → inference worker → R2 MP4 chunks
                                ↘ orchestrator (8080) ← Turso DB ← API ← Admin (Vercel)
```

- **orchestrator** (Go, port 8080): camera registry REST API backed by Turso
- **mediamtx** (port 8554): RTSP server that receives the ffmpeg push
- **inference** (Python + PyAV): buffers decoded frames, flushes 5-second MP4 chunks, uploads to R2
- **admin** (Next.js, Vercel): dashboard to manage cameras and view recordings

## Setup

### 1. VPS first-time init

```bash
scp scripts/vps-init.sh root@YOUR_VPS_IP:/root/vps-init.sh
ssh root@YOUR_VPS_IP "bash /root/vps-init.sh"
```

### 2. Add credentials to GitHub Secrets

Go to `Settings → Secrets and variables → Actions` in your repo and add:

**Backend & R2:**
- `R2_BUCKET_NAME`
- `R2_ACCESS_KEY_ID`
- `R2_SECRET_ACCESS_KEY`
- `R2_ENDPOINT_URL`
- `R2_PUBLIC_URL` (public bucket URL for viewing recordings in admin)
- `ORCHESTRATOR_API_KEY` (protects register/delete/status endpoints)

**Turso DB:**
- `TURSO_DATABASE_URL` (e.g. `libsql://camaron-username.turso.io`)
- `TURSO_AUTH_TOKEN`

**VPS Deploy:**
- `VPS_HOST`
- `VPS_USER`
- `VPS_SSH_KEY`
- `GHCR_TOKEN`

**Admin (Vercel):**
- `VERCEL_TOKEN`
- `VERCEL_ORG_ID`
- `VERCEL_PROJECT_ID`

> The admin dashboard automatically derives `ORCHESTRATOR_URL` from `VPS_HOST` (`http://<VPS_HOST>:8080`). No need to set it manually.

### 3. Apply Turso schema

```bash
turso db shell <your-db-name> < packages/turso/schema.sql
```

> If the `cameras` table already exists without the `resolution` column, run:
> ```bash
> turso db shell <your-db-name> "ALTER TABLE cameras ADD COLUMN resolution TEXT DEFAULT '640x480';"
> ```

### 4. Deploy

```bash
git push origin main
```

GitHub Actions builds images, deploys backend to the VPS, and deploys admin to Vercel.

Verify the stack is up:

```bash
ssh root@YOUR_VPS_IP "curl -s http://localhost:8080/health"
# → {"status":"ok"}
```

---

## Add a camera

### Option A: Admin dashboard (recommended)

1. Open your Vercel admin URL and go to `/cameras`.
2. Fill the **Register new camera** form:
   - **Camera ID:** any unique name (e.g., `macbook`, `backyard`)
   - **Stream URL:** `rtsp://mediamtx:8554/YOUR_CAMERA_ID`
   - **Resolution:** `640×480` (default, lower CPU/RAM) or `1280×720` (higher quality, more CPU)
3. Click **Register camera**.
4. The camera appears in the list as **offline** until you start streaming (see below).

### Option B: Real Mac webcam

```bash
ffmpeg -re -f avfoundation -framerate 30 -video_size 640x480 -pix_fmt yuv420p -i "0" \
  -c:v libx264 -preset ultrafast -tune zerolatency -pix_fmt yuv420p \
  -rtsp_transport tcp \
  -f rtsp rtsp://YOUR_VPS_IP:8554/CAMERA_ID
```

> **Note:** The `-video_size` should match the resolution you selected in the admin panel. Default is `640x480`.

### Option C: Fake / test stream (no camera needed)

Useful for testing the pipeline on a machine without a physical camera:

```bash
ffmpeg -re -f lavfi -i testsrc=size=640x480:rate=30 \
  -pix_fmt yuv420p -c:v libx264 -preset ultrafast -tune zerolatency \
  -rtsp_transport tcp \
  -f rtsp rtsp://YOUR_VPS_IP:8554/CAMERA_ID
```

This generates a synthetic color bars pattern indefinitely.

### Option D: Register via API (curl)

```bash
curl -X POST http://YOUR_VPS_IP:8080/cameras \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $ORCHESTRATOR_API_KEY" \
  -d '{"id":"CAMERA_ID","url":"rtsp://mediamtx:8554/CAMERA_ID","resolution":"640x480"}'
```

### Option E: Remote IP camera with Pi bridge

See `hardware/pi-bridge/` for the full Raspberry Pi Zero 2 W setup guide.

---

## How the flow works

1. **Register** the camera in the admin panel (or via curl).
2. **Start ffmpeg** on the source device to push the stream to the VPS.
3. The **inference worker** polls the orchestrator every 10s, sees the new camera, and opens the RTSP stream.
4. Frames are **sampled at 1 fps**, buffered in memory, and flushed as a **5-second MP4 chunk** to R2.
5. The **admin dashboard** shows the camera as **online** and recordings appear in the camera detail page.

---

## Local Development

Run the stack locally without a VPS:

### 1. Set env vars

```bash
cp .env.example .env
# Edit .env with your Turso and R2 credentials
```

### 2. Start backend services

```bash
cd services && docker compose up --build
```

This brings up orchestrator (8080), MediaMTX (8554), and inference.

### 3. Start admin dashboard

```bash
cd apps/admin && pnpm dev
```

Admin runs on `http://localhost:3000`.

### 4. Register a local camera

```bash
# Stream a fake test pattern (no camera needed)
ffmpeg -re -f lavfi -i testsrc=size=640x480:rate=30 \
  -pix_fmt yuv420p -c:v libx264 -preset ultrafast -tune zerolatency \
  -rtsp_transport tcp \
  -f rtsp rtsp://localhost:8554/testcam

# Register it
curl -X POST http://localhost:8080/cameras \
  -H "Content-Type: application/json" \
  -d '{"id":"testcam","url":"rtsp://mediamtx:8554/testcam","resolution":"640x480"}'
```

---

## API

| Endpoint | Method | Body | Description |
|---|---|---|---|
| `/health` | `GET` | — | Health check |
| `/cameras` | `GET` | — | List all cameras |
| `/cameras` | `POST` | `{"id":"x","url":"rtsp://...","resolution":"640x480"}` | Register a camera |
| `/cameras/{id}` | `GET` | — | Get one camera |
| `/cameras/{id}` | `DELETE` | — | Remove a camera |
| `/camera-status` | `GET` | — | Get all camera statuses |
| `/camera-status` | `POST` | `{"id":"x","event":"chunk_uploaded"}` | Report camera event |
| `/recordings/{id}` | `GET` | `?limit=50` | Get recordings for a camera |

> `POST /cameras` requires `X-API-Key` header if `ORCHESTRATOR_API_KEY` is set.

---

## Troubleshooting

| Problem | Fix |
|---|---|
| `404 Not Found` in inference logs | ffmpeg is not connected. Check ffmpeg is running and the `CAMERA_ID` matches exactly between ffmpeg path and POST body. |
| `Invalid data found when processing input` | Camera URL has a trailing space. Delete + re-register without whitespace. |
| `broken pipe` in ffmpeg | Add `-re` flag so ffmpeg does not flood the server. |
| No `starting camera` in logs | Wait 10s for poll interval. Verify with `curl /cameras`. |
| `r2 upload failed` | Check `.env` on VPS (`cat /opt/camaron/.env`) and GitHub Secrets are correct. |
| Port 8554 refused | Open port 8554 in VPS firewall (`ufw allow 8554`) and cloud provider security group. |
| Turso connection failed | Verify `TURSO_DATABASE_URL` and `TURSO_AUTH_TOKEN` are set and the schema is applied. |
| Camera offline in dashboard but ffmpeg running | Wait 10–15 seconds for inference poll loop. Check `docker logs camaron-inference-1`. |

---

## Project Structure

```
├── apps/
│   ├── admin/              ← Next.js admin dashboard (Vercel)
│   └── docs/
├── packages/
│   ├── turso/              ← DB schema + Go/Python/TS clients
│   ├── typescript-config/
│   └── ui/
├── services/
│   ├── orchestrator/         ← Go REST API (VPS Docker)
│   ├── inference/            ← Python worker (VPS Docker)
│   ├── docker-compose.yml    ← Local stack
│   └── docker-compose.prod.yml ← VPS stack
├── hardware/
│   └── pi-bridge/          ← Raspberry Pi setup guide
└── .github/workflows/ci.yml  ← GitHub Actions CI/CD
```
