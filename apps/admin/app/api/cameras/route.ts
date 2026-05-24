import { addCamera, listCameras } from '@repo/turso';
import { NextResponse } from 'next/server';

export const dynamic = 'force-dynamic';

export async function GET() {
  try {
    const cameras = await listCameras();
    return NextResponse.json(cameras);
  } catch (err) {
    return NextResponse.json(
      { error: err instanceof Error ? err.message : 'Unknown error' },
      { status: 500 },
    );
  }
}

export async function POST(req: Request) {
  try {
    const body = await req.json();
    if (!body.id || !body.url) {
      return NextResponse.json(
        { error: 'id and url required' },
        { status: 400 },
      );
    }
    await addCamera(body.id, body.url, body.resolution);
    return NextResponse.json({ id: body.id, url: body.url, resolution: body.resolution }, { status: 201 });
  } catch (err) {
    return NextResponse.json(
      { error: err instanceof Error ? err.message : 'Unknown error' },
      { status: 409 },
    );
  }
}
