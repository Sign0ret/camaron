package ingest

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/pion/rtp"
)

type RTSPClient struct {
	urlStr    string
	camera    string
	client    *gortsplib.Client
	frames    atomic.Uint64
	drops     atomic.Uint64
	throttled atomic.Uint64

	spsMu sync.RWMutex
	sps   []byte
	pps   []byte

	fpsLimit int
	lastSent time.Time
	stopCh   chan struct{}
	outCh    chan Frame
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
		outCh:    out,
	}, nil
}

func (c *RTSPClient) Run() {
	backoff := time.Second

	for {
		err := c.runSession()

		select {
		case <-c.stopCh:
			log.Printf("[%s] rtsp: stopped", c.camera)
			return
		default:
		}

		log.Printf("[%s] rtsp: session ended, reconnecting in %v (%v)", c.camera, backoff, err)

		select {
		case <-c.stopCh:
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
}

func (c *RTSPClient) runSession() error {
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

	var forma *format.H264
	medi := desc.FindFormat(&forma)
	if medi == nil {
		return fmt.Errorf("no H.264 media found in stream")
	}

	rtpDec, err := forma.CreateDecoder()
	if err != nil {
		return fmt.Errorf("create H.264 decoder: %w", err)
	}

	if forma.SPS != nil {
		c.spsMu.Lock()
		c.sps = forma.SPS
		c.spsMu.Unlock()
		log.Printf("[%s] rtsp: SPS from SDP (%d bytes)", c.camera, len(forma.SPS))
	}
	if forma.PPS != nil {
		c.spsMu.Lock()
		c.pps = forma.PPS
		c.spsMu.Unlock()
		log.Printf("[%s] rtsp: PPS from SDP (%d bytes)", c.camera, len(forma.PPS))
	}

	if _, err := client.Setup(desc.BaseURL, medi, 0, 0); err != nil {
		return fmt.Errorf("setup: %w", err)
	}

	sessionDone := make(chan error, 1)

	client.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
		au, err := rtpDec.Decode(pkt)
		if err != nil {
			if errors.Is(err, rtph264.ErrMorePacketsNeeded) {
				return
			}
			if errors.Is(err, rtph264.ErrNonStartingPacketAndNoPrevious) {
				return
			}
			return
		}

		c.frames.Add(1)

		isRandomAccess := h264.IsRandomAccess(au)

		if !isRandomAccess && time.Since(c.lastSent) < time.Second/time.Duration(c.fpsLimit) {
			c.throttled.Add(1)
			return
		}
		c.lastSent = time.Now()

		var vclNALUs [][]byte
		isIDR := false

		for _, nalu := range au {
			if len(nalu) == 0 {
				continue
			}
			nalType := h264.NALUType(nalu[0] & 0x1F)

			switch nalType {
			case h264.NALUTypeSPS:
				c.spsMu.Lock()
				c.sps = nalu
				c.spsMu.Unlock()
				log.Printf("[%s] rtsp: SPS updated (%d bytes)", c.camera, len(nalu))
			case h264.NALUTypePPS:
				c.spsMu.Lock()
				c.pps = nalu
				c.spsMu.Unlock()
				log.Printf("[%s] rtsp: PPS updated (%d bytes)", c.camera, len(nalu))
			case h264.NALUTypeIDR:
				isIDR = true
				vclNALUs = append(vclNALUs, nalu)
			case h264.NALUTypeNonIDR,
				h264.NALUTypeDataPartitionA,
				h264.NALUTypeDataPartitionB,
				h264.NALUTypeDataPartitionC:
				vclNALUs = append(vclNALUs, nalu)
			}
		}

		if len(vclNALUs) == 0 {
			return
		}

		f := Frame{
			NALUs:     vclNALUs,
			Timestamp: time.Now(),
			CameraID:  c.camera,
			IsIDR:     isIDR,
		}

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

	client.OnDecodeError = func(err error) {
		log.Printf("[%s] rtsp: decode error: %v", c.camera, err)
	}

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

	go func() {
		sessionDone <- client.Wait()
	}()

	for {
		select {
		case <-ticker.C:
			log.Printf("[%s] rtsp: %d frames, %d throttled, %d dropped", c.camera, c.frames.Load(), c.throttled.Load(), c.drops.Load())
		case err := <-sessionDone:
			return err
		case <-c.stopCh:
			return nil
		}
	}
}

func (c *RTSPClient) SPS() []byte {
	c.spsMu.RLock()
	defer c.spsMu.RUnlock()
	return c.sps
}

func (c *RTSPClient) PPS() []byte {
	c.spsMu.RLock()
	defer c.spsMu.RUnlock()
	return c.pps
}

func (c *RTSPClient) Stop() {
	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}
}

func (c *RTSPClient) FramesReceived() uint64  { return c.frames.Load() }
func (c *RTSPClient) FramesDropped() uint64   { return c.drops.Load() }
func (c *RTSPClient) FramesThrottled() uint64 { return c.throttled.Load() }
