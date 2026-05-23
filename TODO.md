# TODO

## 1. Store snapshots in Cloudflare R2 instead of local VPS disk

**Status:** Implemented. Inference worker now uploads every saved JPEG to R2 if credentials are provided.

**What changed:**
- `services/inference/requirements.txt`: added `boto3`
- `services/inference/main.py`: `_upload_to_r2()` helper uploads after each `img.save()`
- `services/docker-compose.prod.yml`: passes R2 env vars to inference container
- `.env.vps.example`: added commented R2 credential template

**To enable R2:**
1. Create a bucket in the Cloudflare R2 dashboard.
2. Generate an S3-compatible API token (Admin Read & Write).
3. Add the 4 values as **GitHub Repository Secrets** (`Settings → Secrets and variables → Actions`):
   - `R2_BUCKET_NAME`
   - `R2_ACCESS_KEY_ID`
   - `R2_SECRET_ACCESS_KEY`
   - `R2_ENDPOINT_URL`
4. Push to `main`. GitHub Actions will write `/opt/camaron/.env` on the VPS automatically and restart inference.

**Remaining:**
- Add R2 public URL to orchestrator camera metadata so the admin app can display images directly.
- Optionally stop saving locally once R2 is proven stable.

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
