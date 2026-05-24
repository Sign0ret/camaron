package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	turso "github.com/Sign0ret/camaron/services/orchestrator/internal/turso"
	"github.com/Sign0ret/camaron/services/orchestrator/internal"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	apiKey := os.Getenv("ORCHESTRATOR_API_KEY")

	allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		allowedOrigins = "*"
	}
	origins := strings.Split(allowedOrigins, ",")

	db, err := turso.Open()
	if err != nil {
		log.Fatalf("turso open: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		log.Fatalf("turso migrate: %v", err)
	}

	manager := internal.NewCameraManager(db)

	checkKey := func(w http.ResponseWriter, r *http.Request) bool {
		if apiKey == "" {
			return true
		}
		if r.Header.Get("X-API-Key") != apiKey {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return false
		}
		return true
	}

	setCORS := func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		for _, o := range origins {
			if o == "*" || o == origin {
				w.Header().Set("Access-Control-Allow-Origin", o)
				break
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
		w.Header().Set("Access-Control-Max-Age", "86400")
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"pong": "verified"})
	})

	mux.HandleFunc("/cameras", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, r)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode(manager.List())
		case http.MethodPost:
			if !checkKey(w, r) {
				return
			}
			var payload struct {
				ID         string `json:"id"`
				URL        string `json:"url"`
				Resolution string `json:"resolution"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
				return
			}
			if payload.ID == "" || payload.URL == "" {
				http.Error(w, `{"error":"id and url required"}`, http.StatusBadRequest)
				return
			}
			if payload.Resolution == "" {
				payload.Resolution = "640x480"
			}
			if err := manager.Add(internal.CameraConfig{
				ID:         payload.ID,
				URL:        payload.URL,
				Resolution: payload.Resolution,
			}); err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"id": payload.ID})
		case http.MethodOptions:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/cameras/", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, r)
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
		case http.MethodOptions:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/stream-configs", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manager.List())
	})

	mux.HandleFunc("/camera-status", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, r)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode(manager.ListStatuses())
		case http.MethodPost:
			if !checkKey(w, r) {
				return
			}
			var payload struct {
				ID    string `json:"id"`
				Event string `json:"event"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
				return
			}
			if payload.ID == "" {
				http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
				return
			}
			switch payload.Event {
			case "chunk_uploaded":
				manager.RecordUpload(payload.ID)
			case "connected":
				manager.SetOnline(payload.ID, true)
			case "disconnected":
				manager.SetOnline(payload.ID, false)
			default:
				manager.SetOnline(payload.ID, true)
			}
			w.WriteHeader(http.StatusNoContent)
		case http.MethodOptions:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/recordings/", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, r)
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		id := r.URL.Path[len("/recordings/"):]
		if id == "" {
			http.Error(w, `{"error":"camera id required"}`, http.StatusBadRequest)
			return
		}
		limitStr := r.URL.Query().Get("limit")
		limit := 50
		if limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
				limit = n
			}
		}
		recs, err := manager.ListRecordings(id, limit)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(recs)
	})

	go func() {
		log.Printf("orchestrator listening on :%s", port)
		if err := http.ListenAndServe(":"+port, mux); err != nil {
			log.Fatalf("http server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")
	log.Println("shutdown complete")
}
