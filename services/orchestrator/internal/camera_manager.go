package internal

import (
	"fmt"
	"sync"
)

// CameraConfig holds the metadata for a single camera stream.
type CameraConfig struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// CameraManager is a thread-safe registry of camera configurations.
type CameraManager struct {
	mu      sync.RWMutex
	cameras map[string]CameraConfig
}

func NewCameraManager() *CameraManager {
	return &CameraManager{
		cameras: make(map[string]CameraConfig),
	}
}

func (m *CameraManager) Add(cfg CameraConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.cameras[cfg.ID]; exists {
		return fmt.Errorf("camera %s already exists", cfg.ID)
	}
	m.cameras[cfg.ID] = cfg
	return nil
}

func (m *CameraManager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.cameras[id]; !exists {
		return fmt.Errorf("camera %s not found", id)
	}
	delete(m.cameras, id)
	return nil
}

func (m *CameraManager) Get(id string) (CameraConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg, ok := m.cameras[id]
	return cfg, ok
}

func (m *CameraManager) List() []CameraConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]CameraConfig, 0, len(m.cameras))
	for _, cfg := range m.cameras {
		out = append(out, cfg)
	}
	return out
}
