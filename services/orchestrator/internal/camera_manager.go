package internal

import (
	"fmt"
	"time"

	turso "github.com/Sign0ret/camaron/services/orchestrator/internal/turso"
)

// CameraConfig holds the metadata for a single camera stream.
type CameraConfig struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// CameraStatus tracks runtime health and activity for a camera.
type CameraStatus struct {
	CameraID       string    `json:"camera_id"`
	Online         bool      `json:"online"`
	LastSeen       time.Time `json:"last_seen"`
	RecordingCount int       `json:"recording_count"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Recording mirrors a single uploaded chunk.
type Recording struct {
	ID         int       `json:"id"`
	Filename   string    `json:"filename"`
	RecordedAt time.Time `json:"recorded_at"`
}

// CameraManager is a thin wrapper around the persistent Turso DB.
type CameraManager struct {
	db *turso.DB
}

func NewCameraManager(db *turso.DB) *CameraManager {
	return &CameraManager{db: db}
}

func (m *CameraManager) Add(cfg CameraConfig) error {
	return m.db.AddCamera(cfg.ID, cfg.URL)
}

func (m *CameraManager) Remove(id string) error {
	return m.db.RemoveCamera(id)
}

func (m *CameraManager) Get(id string) (CameraConfig, bool) {
	cfg, err := m.db.GetCamera(id)
	if err != nil {
		return CameraConfig{}, false
	}
	return CameraConfig{ID: cfg.ID, URL: cfg.URL, CreatedAt: cfg.CreatedAt}, true
}

func (m *CameraManager) List() []CameraConfig {
	items, err := m.db.ListCameras()
	if err != nil {
		return nil
	}
	out := make([]CameraConfig, 0, len(items))
	for _, c := range items {
		out = append(out, CameraConfig{ID: c.ID, URL: c.URL, CreatedAt: c.CreatedAt})
	}
	return out
}

// RecordUpload increments the recording count and updates last seen for a camera.
func (m *CameraManager) RecordUpload(id string) {
	_ = m.db.RecordUpload(id)
}

// SetOnline updates the online status and last seen time for a camera.
func (m *CameraManager) SetOnline(id string, online bool) {
	_ = m.db.SetOnline(id, online)
}

// GetStatus returns the status for a specific camera.
func (m *CameraManager) GetStatus(id string) (CameraStatus, bool) {
	s, err := m.db.GetStatus(id)
	if err != nil {
		return CameraStatus{}, false
	}
	return CameraStatus{
		CameraID:       s.CameraID,
		Online:         s.Online,
		LastSeen:       s.LastSeen,
		RecordingCount: s.RecordingCount,
		UpdatedAt:      s.UpdatedAt,
	}, true
}

// ListStatuses returns all camera statuses keyed by ID.
func (m *CameraManager) ListStatuses() map[string]CameraStatus {
	statuses, err := m.db.ListStatuses()
	if err != nil {
		return nil
	}
	out := make(map[string]CameraStatus, len(statuses))
	for k, v := range statuses {
		out[k] = CameraStatus{
			CameraID:       v.CameraID,
			Online:         v.Online,
			LastSeen:       v.LastSeen,
			RecordingCount: v.RecordingCount,
			UpdatedAt:      v.UpdatedAt,
		}
	}
	return out
}

// ListRecordings returns recent recordings for a camera.
func (m *CameraManager) ListRecordings(cameraID string, limit int) ([]Recording, error) {
	rows, err := m.db.ListRecordings(cameraID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recordings: %w", err)
	}
	out := make([]Recording, 0, len(rows))
	for _, r := range rows {
		out = append(out, Recording{
			ID:         r.ID,
			Filename:   r.Filename,
			RecordedAt: r.RecordedAt,
		})
	}
	return out, nil
}
