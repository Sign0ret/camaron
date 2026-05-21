package recorder

import (
	"bytes"
	"encoding/binary"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Sign0ret/camaron/services/orchestrator/internal/ingest"
	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/codec/h264parser"
	"github.com/nareix/joy4/format/mp4"
)

type Recorder struct {
	camera   *ingest.Camera
	cameraID string
	dir      string
	uploadCh chan<- string
}

func NewRecorder(camera *ingest.Camera, dir string, uploadCh chan<- string) *Recorder {
	return &Recorder{
		camera:   camera,
		cameraID: camera.Status().ID,
		dir:      dir,
		uploadCh: uploadCh,
	}
}

func (r *Recorder) Run() {
	var buffer []ingest.Frame
	skipUntilIDR := true

	for frame := range r.camera.FrameChannel() {
		if skipUntilIDR {
			if !frame.IsIDR {
				continue
			}
			skipUntilIDR = false
		}

		if frame.IsIDR && len(buffer) > 0 {
			r.flush(buffer)
			buffer = nil
		}

		buffer = append(buffer, frame)

		if len(buffer) >= 25 {
			r.flush(buffer)
			buffer = nil
		}
	}

	if len(buffer) > 0 {
		r.flush(buffer)
	}

	log.Printf("[%s] recorder: stopped", r.cameraID)
}

func (r *Recorder) flush(frames []ingest.Frame) {
	if len(frames) == 0 {
		return
	}

	cameraID := frames[0].CameraID

	sps := r.camera.SPS()
	pps := r.camera.PPS()
	if sps == nil || pps == nil {
		log.Printf("[%s] recorder: no SPS/PPS, skipping %d frames", cameraID, len(frames))
		return
	}

	codecData, err := h264parser.NewCodecDataFromSPSAndPPS(sps, pps)
	if err != nil {
		log.Printf("[%s] recorder: codec data error: %v", cameraID, err)
		return
	}

	camDir := filepath.Join(r.dir, cameraID)
	if err := os.MkdirAll(camDir, 0755); err != nil {
		log.Printf("[%s] recorder: mkdir %s: %v", cameraID, camDir, err)
		return
	}

	ts := frames[0].Timestamp
	name := ts.Format("20060102T150405") + ".mp4"
	tmpPath := filepath.Join(camDir, "."+name)
	finalPath := filepath.Join(camDir, name)

	f, err := os.Create(tmpPath)
	if err != nil {
		log.Printf("[%s] recorder: create file: %v", cameraID, err)
		return
	}

	muxer := mp4.NewMuxer(f)
	if err := muxer.WriteHeader([]av.CodecData{codecData}); err != nil {
		f.Close()
		os.Remove(tmpPath)
		log.Printf("[%s] recorder: write header: %v", cameraID, err)
		return
	}

	dts := time.Duration(0)
	frameDur := time.Second / 5

	for _, frame := range frames {
		data := avccPackNALUs(frame.NALUs)
		pkt := av.Packet{
			Idx:        0,
			IsKeyFrame: frame.IsIDR,
			Time:       dts,
			Data:       data,
		}
		if err := muxer.WritePacket(pkt); err != nil {
			f.Close()
			os.Remove(tmpPath)
			log.Printf("[%s] recorder: write packet: %v", cameraID, err)
			return
		}
		dts += frameDur
	}

	if err := muxer.WriteTrailer(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		log.Printf("[%s] recorder: write trailer: %v", cameraID, err)
		return
	}

	f.Close()

	if err := os.Rename(tmpPath, finalPath); err != nil {
		log.Printf("[%s] recorder: rename: %v", cameraID, err)
		os.Remove(tmpPath)
		return
	}

	log.Printf("[%s] recorder: wrote %s (%d frames, %d bytes)",
		cameraID, name, len(frames), fileSize(finalPath))

	if r.uploadCh != nil {
		r.uploadCh <- finalPath
	}
}

func avccPackNALUs(nalus [][]byte) []byte {
	var buf bytes.Buffer
	lenBuf := make([]byte, 4)
	for _, nalu := range nalus {
		binary.BigEndian.PutUint32(lenBuf, uint32(len(nalu)))
		buf.Write(lenBuf)
		buf.Write(nalu)
	}
	return buf.Bytes()
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
