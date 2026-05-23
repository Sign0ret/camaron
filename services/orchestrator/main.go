package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Sign0ret/camaron/services/orchestrator/internal"
)

func main() {
	port := "8080"

	manager := internal.NewCameraManager()

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
		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode(manager.List())
		case http.MethodPost:
			var cfg internal.CameraConfig
			if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
				http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
				return
			}
			if cfg.ID == "" || cfg.URL == "" {
				http.Error(w, `{"error":"id and url required"}`, http.StatusBadRequest)
				return
			}
			if err := manager.Add(cfg); err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusConflict)
				return
			}
			log.Printf("registered camera %s (%s)", cfg.ID, cfg.URL)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(cfg)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/cameras/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := r.URL.Path[len("/cameras/"):]
		if id == "" {
			http.Error(w, `{"error":"camera id required"}`, http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodGet:
			cfg, ok := manager.Get(id)
			if !ok {
				http.Error(w, `{"error":"camera not found"}`, http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(cfg)
		case http.MethodDelete:
			if err := manager.Remove(id); err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
				return
			}
			log.Printf("removed camera %s", id)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/stream-configs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manager.List())
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
	log.Println("shutdown complete")
}
