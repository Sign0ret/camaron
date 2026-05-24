# TODO

## ✅ Done

### R2 MP4 chunk storage
- Inference worker buffers frames in memory and flushes 5-second MP4 chunks to Cloudflare R2.
- Credentials managed via GitHub Repository Secrets, written to VPS `.env` automatically on deploy.

### Admin web app (v1)
- Next.js dashboard with camera list, online/offline status, register/delete forms.
- Recording list per camera with R2 public URLs.
- Camera detail page showing config, status, and recording timeline.

### Skip packet decoding between sample intervals
- Moved the 1-second sample gate **before** `packet.decode()` in `services/inference/main.py`.
- Only ~2 packets per second are decoded per camera (1 keyframe to keep decoder state valid + 1 sampled frame) instead of the full source framerate.
- Cuts per-camera CPU usage by roughly **10–15×**.

---

## 🟢 Tier 1: Reliability

| # | Task | Why |
|---|------|-----|
| 1.1 | **Add Docker resource limits** (`cpus`, `memory`) to the inference container in `docker-compose.prod.yml`. | Prevents runaway CPU or RAM usage from starving the OS / orchestrator / MediaMTX. |
| 1.2 | **Add a healthcheck to the inference container.** | Docker has no visibility today into whether the worker is healthy or deadlocked. A simple check would enable auto-restart. |
| 1.3 | **Stagger chunk flush windows** — add a small per-camera random jitter to `chunk_sec`. | All cameras currently flush every 5 seconds simultaneously (thundering herd), causing CPU spikes and serial encode/upload bottlenecks. |
| 1.4 | **Retry failed R2 uploads.** | `_upload_mp4_to_r2` currently logs and gives up. A transient network hiccup means a lost recording permanently. |

---

## 🟡 Tier 2: Scale (More Cameras on the Same CCX13)

| # | Task | Why |
|---|------|-----|
| 2.1 | **Offload MP4 encoding to a `ThreadPoolExecutor`.** | `libx264` encoding blocks the RTSP reader thread. Parallel encode lets the decoder keep pulling frames while flushing happens in the background. |
| 2.2 | **Shard cameras across two inference replicas.** | Python's GIL wastes the second vCPU. Two inference containers with hash-based camera-ID sharding would double CPU utilization on a 2-core box. |
| 2.3 | **Make resolution configurable per camera** (default 640×480 instead of hardcoded 1280×720). | 480p cuts decode + encode CPU by ~60% and RAM per camera from ~90 MB to ~40 MB. |

---

## 🔵 Tier 3: Product / Admin UX

| # | Task | Why |
|---|------|-----|
| 3.1 | **Auto-refresh camera status** — lightweight polling or SSE on the dashboard. | The admin dashboard is static SSR today. No live visibility into online/offline changes without a manual page reload. |
| 3.2 | **Inline video player for recordings.** | Recordings are currently just raw R2 links. An HTML5 `<video>` tag on the camera detail page would let users watch directly in the dashboard. |
| 3.3 | **Basic auth / password protection for the admin dashboard.** | Orchestrator mutations are protected by `X-API-Key`, but the Vercel-hosted dashboard is completely open to the public. |
| 3.4 | **Live stream preview on camera detail page.** | MediaMTX exposes the RTSP stream. A snapshot or lightweight HLS player on the detail page would be a valuable UX addition. |

---

## 🔴 Tier 4: Future / Outgrow the CCX13

| # | Task | Why |
|---|------|-----|
| 4.1 | **Add a Prometheus-style metrics endpoint.** | Expose active cameras, chunk latency, upload failures, RAM usage, etc. You cannot see when you're hitting capacity limits today. |
| 4.2 | **Move inference off the VPS to a dedicated worker or GPU box.** | The VPS becomes a lightweight RTSP relay + API. Inference scales independently on a beefier machine. |
| 4.3 | **Implement the YOLO + ByteTrack inference pipeline.** | Placeholder exists in `services/inference/main.py`, but this would require a GPU and completely change the per-camera resource math. |

---

## Notes
- Keep infra minimal. One README, one flow.
- Do not over-engineer auth or caching until explicitly needed.
- The current architecture comfortably supports **15–25 cameras** on a Hetzner CCX13 after the decode-skip optimization.
