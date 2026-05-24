# TODO

## ✅ Done

### R2 MP4 chunk storage
- Inference worker buffers frames in memory and flushes 5-second MP4 chunks to Cloudflare R2.
- Credentials managed via GitHub Repository Secrets, written to VPS `.env` automatically on deploy.

---

## In Progress

### Build the Admin web app

**Current state:**
  - `apps/admin` exists as the admin dashboard (Next.js App Router).
- No camera-related UI yet.

**Goal:**
- A lightweight admin dashboard that:
  - Lists registered cameras and their online/offline status.
  - Displays the latest recording for each camera (via R2 public URL).
  - Provides a form to register a new camera without using `curl`.
  - Shows basic stats (last seen, recording count, stream URL).

**Open questions:**
- Authentication: open for now, or add a simple login?
- How does the frontend get recordings? Direct R2 public URL, or proxy through the VPS?
- Real-time updates: WebSocket/SSE, or just poll `/cameras` every few seconds?

**Rough plan:**
1. Scaffold the admin UI in `apps/admin`.
2. Create a cameras list page (`/cameras`) that fetches from the orchestrator.
3. Add a register form (`POST /cameras`).
4. Display the latest recording per camera (R2 URL).
5. Add a camera detail page (`/cameras/[id]`) with recording timeline.
6. Build and deploy (Vercel, or Docker on the same VPS behind a reverse proxy).

---

## Notes
- Keep infra minimal. One README, one flow.
- Do not over-engineer auth or caching until explicitly needed.
