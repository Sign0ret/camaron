import { NextResponse } from "next/server";
import { listCameras, listStatuses } from "@repo/turso";

export async function POST() {
  const [cameras, statuses] = await Promise.all([
    listCameras().catch(() => []),
    listStatuses().catch(() => ({})),
  ]);
  return NextResponse.json({ cameras, statuses });
}
