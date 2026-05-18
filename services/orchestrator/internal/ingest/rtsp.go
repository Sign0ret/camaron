package ingest

import (
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
)

type RTSPClient struct {
	urlStr    string
	camera    string
	client    *gortsplib.Client
	frames    atomic.Uint64
	drops     atomic.Uint64
	throttled atomic.Uint64
	fpsLimit  int
	lastSent  time.Time
	stopCh    chan struct{}
	doneCh    chan struct{}
	outCh     chan Frame
}

func NewRTSPClient(cameraID, rtspURL string, out chan Frame) (*RTSPClient, error) {
	_, err := base.ParseURL(rtspURL)
	if err != nil {
		return nil, fmt.Errorf("invalid RTSP URL: %w", err)
	}
	return &RTSPClient{
		urlStr:   rtspURL,
		camera:   cameraID,
		fpsLimit: 5,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
		outCh:    out,
	}, nil
}

func (c *RTSPClient) Run() error {
	u, err := base.ParseURL(c.urlStr)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}

	client := &gortsplib.Client{
		Scheme: u.Scheme,
		Host:   u.Host,
	}
	c.client = client

	if err := client.Start(); err != nil {
		return fmt.Errorf("start client: %w", err)
	}
	defer client.Close()

	desc, _, err := client.Describe(u)
	if err != nil {
		return fmt.Errorf("describe: %w", err)
	}

	if err := client.SetupAll(desc.BaseURL, desc.Medias); err != nil {
		return fmt.Errorf("setup: %w", err)
	}

	client.OnPacketRTPAny(func(medi *description.Media, _ format.Format, pkt *rtp.Packet) {
		frameType := FrameTypeNonVCL
		if len(pkt.Payload) > 0 {
			nalType := (pkt.Payload[0] & 0x1F)
			if nalType >= 1 && nalType <= 21 {
				frameType = FrameTypeVCL
			}
		}

		now := time.Now()
		if pkt.Timestamp != 0 {
			now = time.Unix(0, int64(pkt.Timestamp))
		}

		f := Frame{
			Data:      pkt.Payload,
			Timestamp: now,
			CameraID:  c.camera,
			Type:      frameType,
		}

		c.frames.Add(1)

		if time.Since(c.lastSent) < time.Second/time.Duration(c.fpsLimit) {
			c.throttled.Add(1)
			return
		}
		c.lastSent = time.Now()

		select {
		case c.outCh <- f:
		default:
			select {
			case <-c.outCh:
				c.drops.Add(1)
			default:
			}
			c.outCh <- f
		}
	})

	if _, err := client.Play(nil); err != nil {
		return fmt.Errorf("play: %w", err)
	}

	log.Printf("[%s] rtsp: streaming started", c.camera)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	go func() {
		<-c.stopCh
		client.Close()
	}()

	for {
		select {
		case <-ticker.C:
			log.Printf("[%s] rtsp: %d frames, %d throttled, %d dropped", c.camera, c.frames.Load(), c.throttled.Load(), c.drops.Load())
		case <-c.doneCh:
			return nil
		}
	}
}

func (c *RTSPClient) Stop() {
	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}
	close(c.doneCh)
}

func (c *RTSPClient) FramesReceived() uint64  { return c.frames.Load() }

func (c *RTSPClient) FramesDropped() uint64   { return c.drops.Load() }

func (c *RTSPClient) FramesThrottled() uint64 { return c.throttled.Load() }
