// Package client manages Rover client connections and their write loops.
package client

import (
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"ntrip-caster/internal/rtcm"
)

// MountRef is the interface that a Client uses to unregister itself when
// its write loop exits. This avoids an import cycle with the mountpoint package.
type MountRef interface {
	RemoveClient(id string)
}

// Client represents a connected Rover that receives RTCM broadcast data.
type Client struct {
	ID          string
	Conn        net.Conn
	Mount       MountRef
	MountName   string
	WriteChan   chan *rtcm.RTCMPacket
	Done        chan struct{}
	slow        int32
	CloseOnce   sync.Once
	ConnectedAt time.Time

	writeTimeout time.Duration
}

// New creates a Client with the given channel buffer size and write timeout.
func New(id string, conn net.Conn, mount MountRef, mountName string, queueSize int, writeTimeout time.Duration) *Client {
	return &Client{
		ID:           id,
		Conn:         conn,
		Mount:        mount,
		MountName:    mountName,
		WriteChan:    make(chan *rtcm.RTCMPacket, queueSize),
		Done:         make(chan struct{}),
		ConnectedAt:  time.Now(),
		writeTimeout: writeTimeout,
	}
}

// KickSlowConsumer closes the connection and signals the write loop to exit.
// It is safe to call multiple times.
func (c *Client) KickSlowConsumer() {
	c.CloseOnce.Do(func() {
		close(c.Done)
		_ = c.Conn.Close()
	})
}

// MarkSlowAndKick atomically marks the client as slow and kicks it.
// The CAS prevents spawning duplicate kick goroutines during broadcast storms.
func (c *Client) MarkSlowAndKick() {
	if atomic.CompareAndSwapInt32(&c.slow, 0, 1) {
		go c.KickSlowConsumer()
	}
}

// WriteLoop drains WriteChan and writes RTCM packets to the TCP connection.
// It must be run as a goroutine; on exit it removes the client from its
// mountpoint. The caller should use a WaitGroup to track this goroutine.
func (c *Client) WriteLoop() {
	defer c.removeFromMount()
	for {
		select {
		case <-c.Done:
			return
		case pkt := <-c.WriteChan:
			if pkt == nil {
				return
			}
			if c.writeTimeout > 0 {
				_ = c.Conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
			}
			_, err := c.Conn.Write(pkt.Data)
			if err != nil {
				slog.Debug("client write error", "client", c.ID, "mount", c.MountName, "err", err)
				c.KickSlowConsumer()
				return
			}
		}
	}
}

func (c *Client) removeFromMount() {
	if c.Mount == nil {
		return
	}
	c.Mount.RemoveClient(c.ID)
}
