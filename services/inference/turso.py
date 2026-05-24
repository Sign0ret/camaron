"""Shared Turso DB client for Camaron services."""

import os
import sqlite3
from datetime import datetime, timezone
from typing import Optional


def _get_connection():
    """Return a sqlite3 connection to Turso via libsql_client if available,
    otherwise fall back to local SQLite for development."""
    url = os.getenv("TURSO_DATABASE_URL", "")
    token = os.getenv("TURSO_AUTH_TOKEN", "")

    try:
        import libsql_experimental

        conn = libsql_experimental.connect(url, auth_token=token)
        return conn
    except ImportError:
        # Fallback to local SQLite for local dev without the libsql driver.
        local_path = os.getenv("TURSO_LOCAL_PATH", "camaron.db")
        return sqlite3.connect(local_path)


def migrate():
    """Run the shared schema."""
    conn = _get_connection()
    schema = """
    CREATE TABLE IF NOT EXISTS cameras (
        id TEXT PRIMARY KEY,
        url TEXT NOT NULL,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );

    CREATE TABLE IF NOT EXISTS camera_status (
        camera_id TEXT PRIMARY KEY,
        online INTEGER DEFAULT 0,
        last_seen DATETIME,
        recording_count INTEGER DEFAULT 0,
        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
    );

    CREATE TABLE IF NOT EXISTS recordings (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        camera_id TEXT NOT NULL,
        filename TEXT NOT NULL,
        recorded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
    );

    CREATE INDEX IF NOT EXISTS idx_recordings_camera ON recordings(camera_id);
    CREATE INDEX IF NOT EXISTS idx_recordings_time ON recordings(recorded_at);
    """
    conn.executescript(schema)
    conn.commit()
    conn.close()


def add_camera(camera_id: str, url: str) -> None:
    conn = _get_connection()
    conn.execute("INSERT INTO cameras (id, url) VALUES (?, ?)", (camera_id, url))
    conn.execute(
        "INSERT INTO camera_status (camera_id, online) VALUES (?, 0)", (camera_id,)
    )
    conn.commit()
    conn.close()


def remove_camera(camera_id: str) -> None:
    conn = _get_connection()
    conn.execute("DELETE FROM cameras WHERE id = ?", (camera_id,))
    conn.commit()
    conn.close()


def list_cameras() -> list[dict]:
    conn = _get_connection()
    rows = conn.execute(
        "SELECT id, url, created_at FROM cameras ORDER BY created_at DESC"
    ).fetchall()
    conn.close()
    return [
        {"id": r[0], "url": r[1], "created_at": r[2]} for r in rows
    ]


def record_upload(camera_id: str, filename: str) -> None:
    now = datetime.now(timezone.utc).isoformat()
    conn = _get_connection()
    conn.execute(
        """
        INSERT INTO camera_status (camera_id, online, last_seen, recording_count, updated_at)
        VALUES (?, 1, ?, 1, ?)
        ON CONFLICT(camera_id) DO UPDATE SET
            online = 1,
            last_seen = excluded.last_seen,
            recording_count = recording_count + 1,
            updated_at = excluded.updated_at
        """,
        (camera_id, now, now),
    )
    conn.execute(
        "INSERT INTO recordings (camera_id, filename) VALUES (?, ?)",
        (camera_id, filename),
    )
    conn.commit()
    conn.close()


def set_online(camera_id: str, online: bool) -> None:
    now = datetime.now(timezone.utc).isoformat()
    conn = _get_connection()
    conn.execute(
        """
        INSERT INTO camera_status (camera_id, online, last_seen, updated_at)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(camera_id) DO UPDATE SET
            online = excluded.online,
            last_seen = excluded.last_seen,
            updated_at = excluded.updated_at
        """,
        (camera_id, 1 if online else 0, now, now),
    )
    conn.commit()
    conn.close()


def get_status(camera_id: str) -> Optional[dict]:
    conn = _get_connection()
    row = conn.execute(
        """
        SELECT camera_id, online, last_seen, recording_count, updated_at
        FROM camera_status WHERE camera_id = ?
        """,
        (camera_id,),
    ).fetchone()
    conn.close()
    if not row:
        return None
    return {
        "camera_id": row[0],
        "online": bool(row[1]),
        "last_seen": row[2],
        "recording_count": row[3],
        "updated_at": row[4],
    }


def list_statuses() -> dict[str, dict]:
    conn = _get_connection()
    rows = conn.execute(
        """
        SELECT camera_id, online, last_seen, recording_count, updated_at
        FROM camera_status
        """
    ).fetchall()
    conn.close()
    return {
        r[0]: {
            "camera_id": r[0],
            "online": bool(r[1]),
            "last_seen": r[2],
            "recording_count": r[3],
            "updated_at": r[4],
        }
        for r in rows
    }


def list_recordings(camera_id: str, limit: int = 50) -> list[dict]:
    conn = _get_connection()
    rows = conn.execute(
        """
        SELECT id, filename, recorded_at FROM recordings
        WHERE camera_id = ? ORDER BY recorded_at DESC LIMIT ?
        """,
        (camera_id, limit),
    ).fetchall()
    conn.close()
    return [
        {"id": r[0], "filename": r[1], "recorded_at": r[2]} for r in rows
    ]
