import { listCameras, listStatuses } from '@repo/turso';
import Link from 'next/link';

export const dynamic = 'force-dynamic';

export default async function Home() {
  const cameras = await listCameras().catch(
    () => [] as { id: string; url: string; created_at: string }[],
  );
  const statuses = await listStatuses().catch(
    () =>
      ({}) as Record<
        string,
        { online: number; last_seen: string | null; recording_count: number }
      >,
  );
  const onlineCount = Object.values(statuses).filter((s) => s.online).length;
  const totalRecordings = Object.values(statuses).reduce(
    (sum, s) => sum + s.recording_count,
    0,
  );

  return (
    <div>
      <h1 className="page-title">Dashboard</h1>

      <div className="grid" style={{ marginBottom: '2rem' }}>
        <div className="card">
          <div className="card-meta">Cameras</div>
          <div className="card-title">{cameras.length}</div>
        </div>
        <div className="card">
          <div className="card-meta">Online</div>
          <div className="card-title" style={{ color: 'var(--success)' }}>
            {onlineCount}
          </div>
        </div>
        <div className="card">
          <div className="card-meta">Recordings</div>
          <div className="card-title">{totalRecordings}</div>
        </div>
      </div>

      <h2 className="section-title">Recent Activity</h2>
      {cameras.length === 0 ? (
        <div className="empty">
          No cameras registered yet.{' '}
          <Link href="/cameras" className="link">
            Add one
          </Link>
          .
        </div>
      ) : (
        <div className="grid">
          {cameras.map((cam) => {
            const status = statuses[cam.id];
            return (
              <div key={cam.id} className="card">
                <div className="card-title">
                  <Link href={`/cameras/${cam.id}`} className="link">
                    {cam.id}
                  </Link>
                  {status?.online ? (
                    <span className="badge badge-online">online</span>
                  ) : (
                    <span className="badge badge-offline">offline</span>
                  )}
                </div>
                <div className="card-meta">{cam.url}</div>
                {status && (
                  <div className="card-meta">
                    {status.recording_count} recordings · Last seen{' '}
                    {status.last_seen
                      ? new Date(status.last_seen).toLocaleString()
                      : 'never'}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
