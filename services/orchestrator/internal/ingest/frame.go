package ingest

import "time"

type FrameType int

const (
	FrameTypeVCL FrameType = iota
	FrameTypeNonVCL
)

type Frame struct {
	Data      []byte
	Timestamp time.Time
	CameraID  string
	Type      FrameType
}
