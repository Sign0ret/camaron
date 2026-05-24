package turso

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

// DB wraps a Turso/libsql connection.
type DB struct {
	conn *sql.DB
}

// CameraConfig mirrors the cameras table.
type CameraConfig struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// CameraStatus mirrors the camera_status table.
type CameraStatus struct {
	CameraID       string    `json:"camera_id"`
	Online         bool      `json:"online"`
	LastSeen       time.Time `json:"last_seen"`
	RecordingCount int       `json:"recording_count"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Open creates a new Turso DB connection from environment.
// Expects TURSO_DATABASE_URL and TURSO_AUTH_TOKEN.
func Open() (*DB, error) {
	url := os.Getenv("TURSO_DATABASE_URL")
	if url == "" {
		return nil, fmt.Errorf("TURSO_DATABASE_URL not set")
	}
	token := os.Getenv("TURSO_AUTH_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TURSO_AUTH_TOKEN not set")
	}

	// libsql-client-go requires the token in the URL for remote connections.
	connStr := fmt.Sprintf("%s?authToken=%s", url, token)
	conn, err := sql.Open("libsql", connStr)
	if err != nil {
		return nil, fmt.Errorf("open turso: %w", err)
	}
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping turso: %w", err)
	}
	return &DB{conn: conn}, nil
}

// Close closes the underlying connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Migrate runs the schema DDL.
func (db *DB) Migrate() error {
	schema := `
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
`
	_, err := db.conn.Exec(schema)
	return err
}

// AddCamera inserts a new camera.
func (db *DB) AddCamera(id, url string) error {
	_, err := db.conn.Exec("INSERT INTO cameras (id, url) VALUES (?, ?)", id, url)
	if err != nil {
		return fmt.Errorf("insert camera: %w", err)
	}
	// seed status row
	_, _ = db.conn.Exec("INSERT INTO camera_status (camera_id, online) VALUES (?, 0)", id)
	return nil
}

// RemoveCamera deletes a camera (cascades to status and recordings).
func (db *DB) RemoveCamera(id string) error {
	_, err := db.conn.Exec("DELETE FROM cameras WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete camera: %w", err)
	}
	return nil
}

// GetCamera fetches a single camera.
func (db *DB) GetCamera(id string) (CameraConfig, error) {
	var cfg CameraConfig
	row := db.conn.QueryRow("SELECT id, url, created_at FROM cameras WHERE id = ?", id)
	err := row.Scan(&cfg.ID, &cfg.URL, &cfg.CreatedAt)
	if err == sql.ErrNoRows {
		return cfg, fmt.Errorf("camera not found")
	}
	if err != nil {
		return cfg, fmt.Errorf("query camera: %w", err)
	}
	return cfg, nil
}

// ListCameras returns all cameras.
func (db *DB) ListCameras() ([]CameraConfig, error) {
	rows, err := db.conn.Query("SELECT id, url, created_at FROM cameras ORDER BY created_at DESC")
	if err != nil {
		return nil, fmt.Errorf("query cameras: %w", err)
	}
	defer rows.Close()

	var out []CameraConfig
	for rows.Next() {
		var cfg CameraConfig
		if err := rows.Scan(&cfg.ID, &cfg.URL, &cfg.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

// RecordUpload increments recording_count and updates last_seen.
func (db *DB) RecordUpload(cameraID string) error {
	now := time.Now().UTC()
	_, err := db.conn.Exec(`
		INSERT INTO camera_status (camera_id, online, last_seen, recording_count, updated_at)
		VALUES (?, 1, ?, 1, ?)
		ON CONFLICT(camera_id) DO UPDATE SET
			online = 1,
			last_seen = excluded.last_seen,
			recording_count = recording_count + 1,
			updated_at = excluded.updated_at
	`, cameraID, now, now)
	if err != nil {
		return fmt.Errorf("record upload: %w", err)
	}
	return nil
}

// SetOnline updates the online flag and last_seen.
func (db *DB) SetOnline(cameraID string, online bool) error {
	now := time.Now().UTC()
	onlineInt := 0
	if online {
		onlineInt = 1
	}
	_, err := db.conn.Exec(`
		INSERT INTO camera_status (camera_id, online, last_seen, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(camera_id) DO UPDATE SET
			online = excluded.online,
			last_seen = excluded.last_seen,
			updated_at = excluded.updated_at
	`, cameraID, onlineInt, now, now)
	if err != nil {
		return fmt.Errorf("set online: %w", err)
	}
	return nil
}

// GetStatus returns the status for a camera.
func (db *DB) GetStatus(cameraID string) (CameraStatus, error) {
	var s CameraStatus
	var onlineInt int
	row := db.conn.QueryRow(`
		SELECT camera_id, online, last_seen, recording_count, updated_at
		FROM camera_status WHERE camera_id = ?`, cameraID)
	err := row.Scan(&s.CameraID, &onlineInt, &s.LastSeen, &s.RecordingCount, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return s, fmt.Errorf("status not found")
	}
	if err != nil {
		return s, fmt.Errorf("query status: %w", err)
	}
	s.Online = onlineInt == 1
	return s, nil
}

// ListStatuses returns all statuses keyed by camera_id.
func (db *DB) ListStatuses() (map[string]CameraStatus, error) {
	rows, err := db.conn.Query(`
		SELECT camera_id, online, last_seen, recording_count, updated_at
		FROM camera_status`)
	if err != nil {
		return nil, fmt.Errorf("query statuses: %w", err)
	}
	defer rows.Close()

	out := make(map[string]CameraStatus)
	for rows.Next() {
		var s CameraStatus
		var onlineInt int
		if err := rows.Scan(&s.CameraID, &onlineInt, &s.LastSeen, &s.RecordingCount, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.Online = onlineInt == 1
		out[s.CameraID] = s
	}
	return out, rows.Err()
}

// InsertRecording logs a new uploaded chunk.
func (db *DB) InsertRecording(cameraID, filename string) error {
	_, err := db.conn.Exec("INSERT INTO recordings (camera_id, filename) VALUES (?, ?)", cameraID, filename)
	if err != nil {
		return fmt.Errorf("insert recording: %w", err)
	}
	return nil
}

// ListRecordings returns recordings for a camera, newest first.
func (db *DB) ListRecordings(cameraID string, limit int) ([]struct {
	ID         int       `json:"id"`
	Filename   string    `json:"filename"`
	RecordedAt time.Time `json:"recorded_at"`
}, error) {
	rows, err := db.conn.Query(`
		SELECT id, filename, recorded_at FROM recordings
		WHERE camera_id = ? ORDER BY recorded_at DESC LIMIT ?`, cameraID, limit)
	if err != nil {
		return nil, fmt.Errorf("query recordings: %w", err)
	}
	defer rows.Close()

	var out []struct {
		ID         int       `json:"id"`
		Filename   string    `json:"filename"`
		RecordedAt time.Time `json:"recorded_at"`
	}
	for rows.Next() {
		var r struct {
			ID         int       `json:"id"`
			Filename   string    `json:"filename"`
			RecordedAt time.Time `json:"recorded_at"`
		}
		if err := rows.Scan(&r.ID, &r.Filename, &r.RecordedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
