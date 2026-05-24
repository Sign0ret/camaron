import { listCameras, listStatuses } from "@repo/turso";
import CameraList from "./CameraList";

export const dynamic = "force-dynamic";

export default async function CamerasPage() {
  const cameras = await listCameras().catch(() => [] as { id: string; url: string; created_at: string }[]);
  const statuses = await listStatuses().catch(() => ({}));

  return <CameraList initialCameras={cameras} initialStatuses={statuses} />;
}
