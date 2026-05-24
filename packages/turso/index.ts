import { createClient, Client } from "@libsql/client";

let _client: Client | null = null;

export function getTursoClient(): Client {
  if (_client) return _client;

  const url = process.env.TURSO_DATABASE_URL;
  const authToken = process.env.TURSO_AUTH_TOKEN;

  if (!url) {
    throw new Error("TURSO_DATABASE_URL not set");
  }

  _client = createClient({ url, authToken });
  return _client;
}

export interface Camera {
  id: string;
  url: string;
  resolution?: string;
  created_at: string;
}

export interface CameraStatus {
  camera_id: string;
  online: number;
  last_seen: string | null;
  recording_count: number;
  updated_at: string;
}

export interface Recording {
  id: number;
  filename: string;
  recorded_at: string;
}

export async function listCameras(): Promise<Camera[]> {
  const db = getTursoClient();
  const rs = await db.execute("SELECT id, url, resolution, created_at FROM cameras ORDER BY created_at DESC");
  return rs.rows.map((r) => ({
    id: r.id as string,
    url: r.url as string,
    resolution: (r.resolution as string) || undefined,
    created_at: r.created_at as string,
  }));
}

export async function addCamera(id: string, url: string, resolution: string = '640x480'): Promise<void> {
  const db = getTursoClient();
  await db.execute({
    sql: "INSERT INTO cameras (id, url, resolution) VALUES (?, ?, ?)",
    args: [id, url, resolution],
  });
  await db.execute({
    sql: "INSERT INTO camera_status (camera_id, online) VALUES (?, 0)",
    args: [id],
  });
}

export async function removeCamera(id: string): Promise<void> {
  const db = getTursoClient();
  await db.execute({
    sql: "DELETE FROM cameras WHERE id = ?",
    args: [id],
  });
}

export async function listStatuses(): Promise<Record<string, CameraStatus>> {
  const db = getTursoClient();
  const rs = await db.execute(
    "SELECT camera_id, online, last_seen, recording_count, updated_at FROM camera_status"
  );
  const out: Record<string, CameraStatus> = {};
  for (const r of rs.rows) {
    out[r.camera_id as string] = {
      camera_id: r.camera_id as string,
      online: r.online as number,
      last_seen: r.last_seen as string | null,
      recording_count: r.recording_count as number,
      updated_at: r.updated_at as string,
    };
  }
  return out;
}

export async function listRecordings(cameraId: string, limit = 50): Promise<Recording[]> {
  const db = getTursoClient();
  const rs = await db.execute({
    sql: "SELECT id, filename, recorded_at FROM recordings WHERE camera_id = ? ORDER BY recorded_at DESC LIMIT ?",
    args: [cameraId, limit],
  });
  return rs.rows.map((r) => ({
    id: r.id as number,
    filename: r.filename as string,
    recorded_at: r.recorded_at as string,
  }));
}
