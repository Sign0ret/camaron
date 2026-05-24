-- Turso schema for Camaron
-- Run: turso db shell camaron-db < schema.sql

CREATE TABLE IF NOT EXISTS cameras (
    id TEXT PRIMARY KEY,
    url TEXT NOT NULL,
    resolution TEXT DEFAULT '640x480',
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
