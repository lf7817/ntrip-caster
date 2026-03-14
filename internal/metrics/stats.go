// Package metrics provides atomic counters for per-mountpoint and global statistics.
package metrics

import "sync/atomic"

// MountStats holds per-mountpoint counters. All fields are updated atomically
// so the broadcast hot-path never needs a lock.
type MountStats struct {
	ClientCount  atomic.Int64
	SourceOnline atomic.Int32
	BytesIn      atomic.Int64
	BytesOut     atomic.Int64
	SlowClients  atomic.Int64
	KickCount    atomic.Int64
}

// Snapshot returns a plain copy suitable for JSON serialisation.
type MountStatsSnapshot struct {
	ClientCount  int64 `json:"client_count"`
	SourceOnline bool  `json:"source_online"`
	BytesIn      int64 `json:"bytes_in"`
	BytesOut     int64 `json:"bytes_out"`
	SlowClients  int64 `json:"slow_clients"`
	KickCount    int64 `json:"kick_count"`
}

// Snapshot takes a point-in-time copy.
func (s *MountStats) Snapshot() MountStatsSnapshot {
	return MountStatsSnapshot{
		ClientCount:  s.ClientCount.Load(),
		SourceOnline: s.SourceOnline.Load() != 0,
		BytesIn:      s.BytesIn.Load(),
		BytesOut:     s.BytesOut.Load(),
		SlowClients:  s.SlowClients.Load(),
		KickCount:    s.KickCount.Load(),
	}
}
