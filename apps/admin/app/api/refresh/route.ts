import { listCameras, listStatuses } from '@repo/turso';
import { NextResponse } from 'next/server';

export async function POST() {
  const [cameras, statuses] = await Promise.all([
    listCameras().catch(() => []),
    listStatuses().catch(() => ({})),
  ]);
  return NextResponse.json({ cameras, statuses });
}
