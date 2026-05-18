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
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	manager := internal.NewCameraManager()

	cameraURLs := os.Getenv("CAMERA_URLS")
	if cameraURLs != "" {
		for _, entry := range strings.Split(cameraURLs, ",") {
			parts := strings.SplitN(entry, "=", 2)
			if len(parts) != 2 {
				log.Printf("invalid camera entry: %s", entry)
				continue
			}
			id, url := parts[0], parts[1]
			if err := manager.AddCamera(id, url); err != nil {
				log.Printf("failed to add camera %s: %v", id, err)
			}
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
		log.Printf("orchestrator listening on :%s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
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
