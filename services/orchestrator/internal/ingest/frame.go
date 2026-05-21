package ingest

import "time"

type Frame struct {
	NALUs     [][]byte
	Timestamp time.Time
	CameraID  string
	IsIDR     bool
}
