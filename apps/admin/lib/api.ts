export const R2_PUBLIC_URL = process.env.R2_PUBLIC_URL || '';

export interface Camera {
  id: string;
  url: string;
  resolution?: string;
  created_at?: string;
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

export async function registerCamera(
  id: string,
  url: string,
  resolution = '640x480',
): Promise<Camera> {
  const res = await fetch('/api/cameras', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id, url, resolution }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: 'Unknown error' }));
    throw new Error(err.error || 'Failed to register camera');
  }
  return res.json();
}

export async function deleteCamera(id: string): Promise<void> {
  const res = await fetch(`/api/cameras/${id}`, { method: 'DELETE' });
  if (!res.ok) throw new Error('Failed to delete camera');
}

export function getRecordingUrl(cameraId: string, filename: string): string {
  if (!R2_PUBLIC_URL) return '';
  return `${R2_PUBLIC_URL}/${cameraId}/${filename}`;
}
