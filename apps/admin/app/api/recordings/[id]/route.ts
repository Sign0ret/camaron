import { NextResponse } from "next/server";
import { listRecordings } from "@repo/turso";

export const dynamic = "force-dynamic";

export async function GET(_req: Request, { params }: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await params;
    const { searchParams } = new URL(_req.url);
    const limit = parseInt(searchParams.get("limit") || "50", 10);
    const recordings = await listRecordings(id, limit);
    return NextResponse.json(recordings);
  } catch (err: any) {
    return NextResponse.json({ error: err.message }, { status: 500 });
  }
}
