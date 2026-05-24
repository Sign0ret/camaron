"use client";

import { useState } from "react";
import {
  registerCamera,
  deleteCamera,
  type Camera,
  type CameraStatus,
} from "@/lib/api";
import Link from "next/link";

export default function CameraList({
  initialCameras,
  initialStatuses,
}: {
  initialCameras: Camera[];
  initialStatuses: Record<string, CameraStatus>;
}) {
  const [cameras, setCameras] = useState<Camera[]>(initialCameras);
  const [statuses, setStatuses] = useState<Record<string, CameraStatus>>(
    initialStatuses
  );
  const [form, setForm] = useState({ id: "", url: "" });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");

  const refresh = async () => {
    try {
      const res = await fetch("/api/refresh", { method: "POST" });
      const data = await res.json();
      setCameras(data.cameras);
      setStatuses(data.statuses);
    } catch {
      // silent
    }
  };

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError("");
    setSuccess("");
    try {
      await registerCamera(form.id, form.url);
      setSuccess(`Camera "${form.id}" registered.`);
      setForm({ id: "", url: "" });
      await refresh();
    } catch (err: any) {
      setError(err.message || "Failed to register camera");
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm(`Delete camera "${id}"?`)) return;
    setError("");
    try {
      await deleteCamera(id);
      setCameras((prev) => prev.filter((c) => c.id !== id));
    } catch (err: any) {
      setError(err.message || "Failed to delete camera");
    }
  };

  return (
    <div>
      <h1 className="page-title">Cameras</h1>

      {error && (
        <div className="error" style={{ marginBottom: "1rem" }}>
          {error}
        </div>
      )}
      {success && (
        <div className="success" style={{ marginBottom: "1rem" }}>
          {success}
        </div>
      )}

      <form
        onSubmit={handleRegister}
        className="form"
        style={{ marginBottom: "2rem" }}
      >
        <h2 className="section-title" style={{ marginTop: 0 }}>
          Register new camera
        </h2>
        <div className="form-group">
          <label htmlFor="id">Camera ID</label>
          <input
            id="id"
            type="text"
            placeholder="e.g. backyard"
            value={form.id}
            onChange={(e) =>
              setForm((f) => ({ ...f, id: e.target.value }))
            }
            required
          />
        </div>
        <div className="form-group">
          <label htmlFor="url">Stream URL</label>
          <input
            id="url"
            type="text"
            placeholder="rtsp://... or device://0"
            value={form.url}
            onChange={(e) =>
              setForm((f) => ({ ...f, url: e.target.value }))
            }
            required
          />
        </div>
        <button
          type="submit"
          className="btn btn-primary"
          disabled={loading}
        >
          {loading ? "Registering..." : "Register camera"}
        </button>
      </form>

      <h2 className="section-title">Registered cameras</h2>
      {cameras.length === 0 ? (
        <div className="empty">No cameras registered.</div>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>ID</th>
              <th>Status</th>
              <th>Recordings</th>
              <th>Last seen</th>
              <th style={{ textAlign: "right" }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {cameras.map((cam) => {
              const status = statuses[cam.id];
              return (
                <tr key={cam.id}>
                  <td>
                    <Link href={`/cameras/${cam.id}`} className="link">
                      {cam.id}
                    </Link>
                    <div
                      className="card-meta"
                      style={{ marginTop: "0.25rem" }}
                    >
                      {cam.url}
                    </div>
                  </td>
                  <td>
                    {status?.online ? (
                      <span className="badge badge-online">online</span>
                    ) : (
                      <span className="badge badge-offline">offline</span>
                    )}
                  </td>
                  <td>{status?.recording_count ?? 0}</td>
                  <td>
                    {status?.last_seen
                      ? new Date(status.last_seen).toLocaleString()
                      : "—"}
                  </td>
                  <td style={{ textAlign: "right" }}>
                    <button
                      className="btn btn-danger btn-sm"
                      onClick={() => handleDelete(cam.id)}
                    >
                      Delete
                    </button>
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
