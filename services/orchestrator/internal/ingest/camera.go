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
		output: make(chan Frame, 5),
	}
}

func (c *Camera) Start() error {
	client, err := NewRTSPClient(c.id, c.url, c.output)
	if err != nil {
		return err
	}
	c.client = client

	go func() {
		if err := client.Run(); err != nil {
			log.Printf("[%s] camera error: %v", c.id, err)
		}
	}()
	return nil
}

func (c *Camera) Stop() {
	if c.client != nil {
		c.client.Stop()
	}
	close(c.output)
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
