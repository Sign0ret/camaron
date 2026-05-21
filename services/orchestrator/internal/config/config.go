package config

import (
	"encoding/json"
	"os"
	"strconv"
)

type CameraConfig struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type Config struct {
	Port string

	Cameras []CameraConfig

	S3Endpoint     string
	S3Bucket       string
	S3AccessKeyID  string
	S3SecretKey    string
	S3Region       string
	S3UsePathStyle bool
	S3Workers      int
	S3QueueSize    int

	RecordingDir  string
	SegmentFrames int
	FPSLimit      int
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:          env("PORT", "8080"),
		SegmentFrames: envInt("SEGMENT_FRAMES", 25),
		FPSLimit:      envInt("FPS_LIMIT", 5),

		S3Endpoint:     env("S3_ENDPOINT", "http://minio:9000"),
		S3Bucket:       env("S3_BUCKET", "camaron"),
		S3AccessKeyID:  env("S3_ACCESS_KEY_ID", "minioadmin"),
		S3SecretKey:    env("S3_SECRET_ACCESS_KEY", "minioadmin"),
		S3Region:       env("S3_REGION", "us-east-1"),
		S3UsePathStyle: envBool("S3_USE_PATH_STYLE", true),
		S3Workers:      envInt("UPLOAD_WORKERS", 10),
		S3QueueSize:    envInt("UPLOAD_QUEUE_SIZE", 500),

		RecordingDir: env("RECORDING_DIR", "/tmp/camaron/recordings"),
	}

	configFile := os.Getenv("CAMERAS_CONFIG")
	if configFile != "" {
		cameras, err := loadCamerasFile(configFile)
		if err != nil {
			return nil, err
		}
		cfg.Cameras = cameras
	} else {
		cfg.Cameras = parseCameraURLs(os.Getenv("CAMERA_URLS"))
	}

	return cfg, nil
}

func loadCamerasFile(path string) ([]CameraConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Cameras []CameraConfig `json:"cameras"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Cameras, nil
}

func parseCameraURLs(raw string) []CameraConfig {
	if raw == "" {
		return nil
	}
	var cameras []CameraConfig
	for {
		idx := -1
		for i := 0; i < len(raw); i++ {
			if raw[i] == ',' {
				idx = i
				break
			}
		}
		entry := raw
		if idx >= 0 {
			entry = raw[:idx]
			raw = raw[idx+1:]
		} else {
			raw = ""
		}
		parts := splitN(entry, "=", 2)
		if len(parts) == 2 {
			cameras = append(cameras, CameraConfig{ID: parts[0], URL: parts[1]})
		}
		if raw == "" {
			break
		}
	}
	return cameras
}

func splitN(s, sep string, n int) []string {
	parts := []string{}
	for i := 0; i < n-1 && len(s) > 0; i++ {
		idx := -1
		for j := 0; j <= len(s)-len(sep); j++ {
			if s[j:j+len(sep)] == sep {
				idx = j
				break
			}
		}
		if idx < 0 {
			parts = append(parts, s)
			s = ""
		} else {
			parts = append(parts, s[:idx])
			s = s[idx+len(sep):]
		}
	}
	if len(s) > 0 {
		parts = append(parts, s)
	}
	for len(parts) < n {
		parts = append(parts, "")
	}
	return parts
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
