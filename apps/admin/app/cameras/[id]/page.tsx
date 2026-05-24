import { listCameras, listRecordings, listStatuses } from '@repo/turso';
import Link from 'next/link';

export const dynamic = 'force-dynamic';

interface Props {
  params: Promise<{ id: string }>;
}

export default async function CameraDetailPage({ params }: Props) {
  const { id } = await params;

  const cameras = await listCameras().catch(() => []);
  const camera = cameras.find((c) => c.id === id);

  const statuses = await listStatuses().catch(
    () =>
      ({}) as Record<
        string,
        { online: number; last_seen: string | null; recording_count: number }
      >,
  );
  const status = id in statuses ? statuses[id] : undefined;

  const recordings = await listRecordings(id, 20).catch(() => []);

  const R2_PUBLIC_URL = process.env.R2_PUBLIC_URL || '';

  if (!camera) {
    return (
      <div>
        <h1 className="page-title">Camera not found</h1>
        <p>
          <Link href="/cameras" className="link">
            Back to cameras
          </Link>
        </p>
      </div>
    );
  }

  return (
    <div>
      <div style={{ marginBottom: '1.5rem' }}>
        <Link href="/cameras" className="link">
          ← Cameras
        </Link>
      </div>

      <h1 className="page-title">{camera.id}</h1>

      <div className="card" style={{ marginBottom: '2rem' }}>
        <div className="card-title">Configuration</div>
        <div className="card-meta">Stream URL: {camera.url}</div>
        <div className="card-meta">Resolution: {camera.resolution || '640x480'}</div>
        <div className="card-meta">
          Created:{' '}
          {camera.created_at
            ? new Date(camera.created_at).toLocaleString()
            : '—'}
        </div>
      </div>

      {status && (
        <div className="grid" style={{ marginBottom: '2rem' }}>
          <div className="card">
            <div className="card-meta">Status</div>
            <div className="card-title">
              {status.online ? (
                <span style={{ color: 'var(--success)' }}>Online</span>
              ) : (
                <span style={{ color: 'var(--danger)' }}>Offline</span>
              )}
            </div>
          </div>
          <div className="card">
            <div className="card-meta">Recordings</div>
            <div className="card-title">{status.recording_count}</div>
          </div>
          <div className="card">
            <div className="card-meta">Last seen</div>
            <div className="card-title">
              {status.last_seen
                ? new Date(status.last_seen).toLocaleString()
                : '—'}
            </div>
          </div>
        </div>
      )}

      <h2 className="section-title">Recent Recordings</h2>
      {recordings.length === 0 ? (
        <div className="empty">No recordings yet.</div>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>Filename</th>
              <th>Recorded at</th>
              <th>Link</th>
            </tr>
          </thead>
          <tbody>
            {recordings.map((rec) => {
              const url = R2_PUBLIC_URL
                ? `${R2_PUBLIC_URL}/${id}/${rec.filename}`
                : '';
              return (
                <tr key={rec.id}>
                  <td>{rec.filename}</td>
                  <td>{new Date(rec.recorded_at).toLocaleString()}</td>
                  <td>
                    {url ? (
                      <a
                        href={url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="link"
                      >
                        View
                      </a>
                    ) : (
                      '—'
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
  );
}
