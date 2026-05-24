import { removeCamera } from '@repo/turso';
import { NextResponse } from 'next/server';

export const dynamic = 'force-dynamic';

export async function DELETE(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  try {
    const { id } = await params;
    await removeCamera(id);
    return new NextResponse(null, { status: 204 });
  } catch (err) {
    return NextResponse.json(
      { error: err instanceof Error ? err.message : 'Unknown error' },
      { status: 500 },
    );
  }
}
