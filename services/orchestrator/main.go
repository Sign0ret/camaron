package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Sign0ret/camaron/services/orchestrator/internal"
	"github.com/Sign0ret/camaron/services/orchestrator/internal/config"
	"github.com/Sign0ret/camaron/services/orchestrator/internal/uploader"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if len(cfg.Cameras) == 0 {
		log.Println("no cameras configured")
	}

	upl, err := uploader.NewUploader(uploader.Config{
		Endpoint:     cfg.S3Endpoint,
		Bucket:       cfg.S3Bucket,
		AccessKeyID:  cfg.S3AccessKeyID,
		SecretKey:    cfg.S3SecretKey,
		Region:       cfg.S3Region,
		UsePathStyle: cfg.S3UsePathStyle,
		Workers:      cfg.S3Workers,
		QueueSize:    cfg.S3QueueSize,
		RecordingDir: cfg.RecordingDir,
	})
	if err != nil {
		log.Printf("uploader: init failed: %v (uploads disabled)", err)
		upl = nil
	}

	var uploadCh chan<- string
	if upl != nil {
		if err := upl.Start(); err != nil {
			log.Printf("uploader: start failed: %v (uploads disabled)", err)
			upl = nil
		} else {
			uploadCh = upl.Channel()
			defer upl.Stop()
		}
	}

	manager := internal.NewCameraManager(cfg, uploadCh)

	for _, cam := range cfg.Cameras {
		if err := manager.AddCamera(cam.ID, cam.URL); err != nil {
			log.Printf("failed to add camera %s: %v", cam.ID, err)
		}
	}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"pong": "verified"})
	})

	http.HandleFunc("/cameras", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manager.List())
	})

	http.HandleFunc("/camera/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := strings.TrimPrefix(r.URL.Path, "/camera/")
		id = strings.TrimSuffix(id, "/status")
		if id == "" {
			http.Error(w, `{"error":"camera id required"}`, http.StatusBadRequest)
			return
		}
		status, ok := manager.Status(id)
		if !ok {
			http.Error(w, `{"error":"camera not found"}`, http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(status)
	})

	go func() {
		log.Printf("orchestrator listening on :%s", cfg.Port)
		if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
			log.Fatalf("http server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")
	manager.Shutdown()
	log.Println("shutdown complete")
}
