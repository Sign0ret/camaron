import { NextResponse } from "next/server";
import { listStatuses } from "@repo/turso";

export const dynamic = "force-dynamic";

export async function GET() {
  try {
    const statuses = await listStatuses();
    return NextResponse.json(statuses);
  } catch (err: any) {
    return NextResponse.json({ error: err.message }, { status: 500 });
  }
}
