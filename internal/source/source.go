// Package source manages Base Station (NTRIP Source) connections.
package source

import (
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"ntrip-caster/internal/metrics"
	"ntrip-caster/internal/mountpoint"
	"ntrip-caster/internal/rtcm"
)

// Source represents a connected NTRIP Base Station that pushes RTCM data.
type Source struct {
	ID        string
	UserID    int64 // user who established this connection (0 if no auth)
	Conn      net.Conn
	Mount     *mountpoint.MountPoint
	Done      chan struct{}
	CloseOnce sync.Once
	StartTime time.Time
	BytesIn   atomic.Int64 // bytes received from this source
}

// New creates a new Source and registers it on the given mountpoint.
// Returns nil if the mountpoint already has a source.
func New(id string, userID int64, conn net.Conn, mp *mountpoint.MountPoint) *Source {
	s := &Source{
		ID:        id,
		UserID:    userID,
		Conn:      conn,
		Mount:     mp,
		Done:      make(chan struct{}),
		StartTime: time.Now(),
	}

	info := &mountpoint.SourceInfo{
		ID:        id,
		UserID:    userID,
		Done:      s.Done,
		Stop:      func() { s.Close() },
		BytesIn:   &s.BytesIn,
		StartTime: s.StartTime,
	}
	if !mp.SetSource(info) {
		return nil
	}
	return s
}

// Close shuts down the source connection. Safe to call multiple times.
func (s *Source) Close() {
	s.CloseOnce.Do(func() {
		close(s.Done)
		_ = s.Conn.Close()
	})
}

// ReadLoop reads RTCM data from the source connection, frames it, and
// broadcasts complete packets to the mountpoint. It blocks until the
// connection is closed or an error occurs.
func (s *Source) ReadLoop() {
	defer func() {
		s.Mount.ClearSource(s.ID)
		s.Close()
	}()

	framer := &rtcm.RTCMFramer{}
	buf := make([]byte, 4096)
	for {
		select {
		case <-s.Done:
			return
		default:
		}

		n, err := s.Conn.Read(buf)
		if err != nil {
			slog.Debug("source read error", "source", s.ID, "err", err)
			return
		}

		s.BytesIn.Add(int64(n))
		s.Mount.Stats.BytesIn.Add(int64(n))
		packets := framer.Push(buf[:n])
		for _, pkt := range packets {
			// 检查是否为 1005 报文
			if rtcm.ExtractMsgType(pkt) == 1005 {
				// 打印原始报文数据（用于调试）
				slog.Info("received 1005 packet",
					"source", s.ID,
					"mount", s.Mount.Name,
					"raw_hex", fmt.Sprintf("%X", pkt.Data),
					"len", len(pkt.Data))

				pos, err := rtcm.Decode1005(pkt)
				if err == nil && pos != nil {
					slog.Info("1005 decode success",
						"source", s.ID,
						"mount", s.Mount.Name,
						"lat", pos.Latitude,
						"lon", pos.Longitude,
						"height", pos.Height)
					s.Mount.UpdateAntennaPosition(pos)
				} else if err != nil {
					slog.Warn("1005 decode failed",
						"source", s.ID,
						"mount", s.Mount.Name,
						"raw_hex", fmt.Sprintf("%X", pkt.Data),
						"err", err)
				}
			}

			s.Mount.Broadcast(pkt)
			addBytesOut(&s.Mount.Stats, pkt)
		}
	}
}

func addBytesOut(stats *metrics.MountStats, pkt *rtcm.RTCMPacket) {
	cc := stats.ClientCount.Load()
	if cc > 0 {
		stats.BytesOut.Add(int64(len(pkt.Data)) * cc)
	}
}
