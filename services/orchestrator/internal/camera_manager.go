package internal

import (
	"fmt"
	"log"
	"sync"

	"github.com/Sign0ret/camaron/services/orchestrator/internal/config"
	"github.com/Sign0ret/camaron/services/orchestrator/internal/ingest"
	"github.com/Sign0ret/camaron/services/orchestrator/internal/recorder"
)

type CameraManager struct {
	mu       sync.RWMutex
	cameras  map[string]*ingest.Camera
	cfg      *config.Config
	uploadCh chan<- string
}

func NewCameraManager(cfg *config.Config, uploadCh chan<- string) *CameraManager {
	return &CameraManager{
		cameras:  make(map[string]*ingest.Camera),
		cfg:      cfg,
		uploadCh: uploadCh,
	}
}

func (m *CameraManager) AddCamera(id, url string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.cameras[id]; exists {
		return fmt.Errorf("camera %s already exists", id)
	}

	cam := ingest.NewCamera(id, url)
	if err := cam.Start(); err != nil {
		return fmt.Errorf("start camera %s: %w", id, err)
	}

	m.cameras[id] = cam
	log.Printf("manager: camera %s added (%s)", id, url)

	rec := recorder.NewRecorder(cam, m.cfg.RecordingDir, m.uploadCh)
	go rec.Run()

	log.Printf("manager: recorder started for %s", id)
	return nil
}

func (m *CameraManager) RemoveCamera(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cam, exists := m.cameras[id]
	if !exists {
		return fmt.Errorf("camera %s not found", id)
	}

	cam.Stop()
	delete(m.cameras, id)
	log.Printf("manager: camera %s removed", id)
	return nil
}

func (m *CameraManager) Status(id string) (ingest.CameraStatus, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cam, exists := m.cameras[id]
	if !exists {
		return ingest.CameraStatus{}, false
	}
	return cam.Status(), true
}

func (m *CameraManager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.cameras))
	for id := range m.cameras {
		ids = append(ids, id)
	}
	return ids
}

func (m *CameraManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, cam := range m.cameras {
		cam.Stop()
		log.Printf("manager: shutdown camera %s", id)
	}
	m.cameras = make(map[string]*ingest.Camera)
}
