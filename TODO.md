# TODO

## 1. Store snapshots in Cloudflare R2 instead of local VPS disk

**Current state:**
- Inference worker saves JPEG snapshots to `/data/camaron/snapshots/{cameraId}/` on the VPS filesystem.
- Users `rsync` them down to their Mac.

**Goal:**
- Upload every snapshot to a Cloudflare R2 bucket.
- Replace local-only storage so snapshots survive VPS disk loss and are accessible via HTTPS URL.

**Open questions / decisions:**
- R2 bucket name and public/private access settings.
- Credentials strategy: env vars injected via `.env` (already supported) vs. IAM-style tokens.
- File naming / path structure in R2. Keep the same `cameraId/frame_YYYYMMDD_HHMMSS.jpg` convention?
- Do we still keep a local cache, or upload-only?
- Should the orchestrator return presigned URLs for the latest snapshot per camera?

**Rough plan:**
1. Add `boto3` to `services/inference/requirements.txt` (R2 is S3-compatible).
2. Update `services/inference/main.py` to upload each saved JPEG to R2 after writing locally.
3. Add R2 env vars to `.env.vps.example` and `docker-compose.prod.yml`.
4. Verify uploads via R2 dashboard / `aws s3 ls` equivalent.
5. (Later) Optionally remove local disk storage entirely if R2 is the source of truth.

---

## 2. Build the Admin web app

**Current state:**
- `apps/web` exists as a default Next.js project created with `create-next-app`.
- No camera-related UI yet.

**Goal:**
- A lightweight admin dashboard that:
  - Lists registered cameras and their online/offline status.
  - Displays the latest snapshot for each camera (via R2 URL or local path).
  - Provides a form to register a new camera without using `curl`.
  - Shows basic stats (e.g., last seen, snapshot count, stream URL).

**Open questions / decisions:**
- Next.js App Router vs. Pages Router? (Currently App Router by default.)
- Styling: Tailwind is already in Next.js by default. Keep it simple.
- Authentication: do we need any login for the admin panel, or is it open for now?
- How does the frontend get snapshot images? Direct R2 public URL, or via an API route that proxies/verifies?
- Do we add a WebSocket/SSE endpoint to push camera updates, or just poll `/cameras`?

**Rough plan:**
1. Scaffold the admin UI in `apps/web`.
2. Create a cameras list page (`/cameras`) that fetches from `http://YOUR_VPS_IP:8080/cameras`.
3. Add a register form (`POST /cameras`).
4. Display the latest snapshot per camera (placeholder image first, then wire R2 URL).
5. Add a camera detail page (`/cameras/[id]`) with recent snapshot timeline.
6. Build and deploy the web app (Vercel, or Docker on the same VPS behind a reverse proxy).

---

## Notes
- Keep infra minimal. One README, one flow.
- Do not over-engineer auth or caching until explicitly needed.
