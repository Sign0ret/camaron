package ingest

import "log"

type CameraStatus struct {
	ID              string `json:"id"`
	URL             string `json:"url"`
	Running         bool   `json:"running"`
	FramesIn        uint64 `json:"frames_in"`
	FramesThrottled uint64 `json:"frames_throttled"`
	FramesDropped   uint64 `json:"frames_dropped"`
}

type Camera struct {
	id     string
	url    string
	client *RTSPClient
	output chan Frame
}

func NewCamera(id, url string) *Camera {
	return &Camera{
		id:     id,
		url:    url,
		output: make(chan Frame, 30),
	}
}

func (c *Camera) Start() error {
	client, err := NewRTSPClient(c.id, c.url, c.output)
	if err != nil {
		return err
	}
	c.client = client

	go func() {
		client.Run()
		close(c.output)
		log.Printf("[%s] camera: stopped", c.id)
	}()
	return nil
}

func (c *Camera) Stop() {
	if c.client != nil {
		c.client.Stop()
	}
}

func (c *Camera) Status() CameraStatus {
	running := c.client != nil
	var framesIn, framesThrottled, framesDropped uint64
	if c.client != nil {
		framesIn = c.client.FramesReceived()
		framesThrottled = c.client.FramesThrottled()
		framesDropped = c.client.FramesDropped()
	}
	return CameraStatus{
		ID:              c.id,
		URL:             c.url,
		Running:         running,
		FramesIn:        framesIn,
		FramesThrottled: framesThrottled,
		FramesDropped:   framesDropped,
	}
}

func (c *Camera) FrameChannel() <-chan Frame {
	return c.output
}

func (c *Camera) SPS() []byte {
	if c.client != nil {
		return c.client.SPS()
	}
	return nil
}

func (c *Camera) PPS() []byte {
	if c.client != nil {
		return c.client.PPS()
	}
	return nil
}
