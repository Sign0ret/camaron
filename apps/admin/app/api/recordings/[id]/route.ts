import { listRecordings } from '@repo/turso';
import { NextResponse } from 'next/server';

export const dynamic = 'force-dynamic';

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  try {
    const { id } = await params;
    const { searchParams } = new URL(_req.url);
    const limit = Number.parseInt(searchParams.get('limit') || '50', 10);
    const recordings = await listRecordings(id, limit);
    return NextResponse.json(recordings);
  } catch (err) {
    return NextResponse.json(
      { error: err instanceof Error ? err.message : 'Unknown error' },
      { status: 500 },
    );
  }
}
