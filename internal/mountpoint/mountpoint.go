// Package mountpoint manages NTRIP mountpoints and RTCM broadcast to clients.
package mountpoint

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"ntrip-caster/internal/client"
	"ntrip-caster/internal/metrics"
	"ntrip-caster/internal/rtcm"
)

// MountPoint represents a single NTRIP mountpoint with an optional source
// and zero or more rover clients.
//
// The broadcast hot-path uses an atomic.Value snapshot of []*client.Client
// so readers never take a lock. Writers (add/remove client) hold mu and
// rebuild the snapshot.
type MountPoint struct {
	Name         string
	Description  string
	Format       string
	Enabled      bool
	WriteQueue   int
	WriteTimeout time.Duration

	mu          sync.Mutex
	source      *SourceInfo
	clientsByID map[string]*client.Client
	snapshot    atomic.Value // []*client.Client

	Stats metrics.MountStats
}

// SourceInfo is a lightweight view of the currently connected source,
// kept inside MountPoint to avoid import cycles with the source package.
type SourceInfo struct {
	ID   string
	Done chan struct{}
	Stop func() // called to close the source connection
}

// NewMountPoint creates an enabled mountpoint with the given defaults.
func NewMountPoint(name, description, format string, writeQueue int, writeTimeout time.Duration) *MountPoint {
	mp := &MountPoint{
		Name:         name,
		Description:  description,
		Format:       format,
		Enabled:      true,
		WriteQueue:   writeQueue,
		WriteTimeout: writeTimeout,
		clientsByID:  make(map[string]*client.Client),
	}
	mp.snapshot.Store(([]*client.Client)(nil))
	return mp
}

// SetSource registers a source on this mountpoint. Returns false if a source
// is already connected.
func (m *MountPoint) SetSource(info *SourceInfo) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.source != nil {
		return false
	}
	m.source = info
	m.Stats.SourceOnline.Store(1)
	slog.Info("source connected", "mount", m.Name, "source", info.ID)
	return true
}

// ClearSource removes the current source. It is a no-op if src does not
// match the current source ID (prevents a stale goroutine from clearing a
// new source).
func (m *MountPoint) ClearSource(srcID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.source == nil || m.source.ID != srcID {
		return
	}
	m.source = nil
	m.Stats.SourceOnline.Store(0)
	slog.Info("source disconnected", "mount", m.Name, "source", srcID)
}

// HasSource reports whether a source is currently connected.
func (m *MountPoint) HasSource() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.source != nil
}

// SourceID returns the ID of the current source, or "" if none.
func (m *MountPoint) SourceID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.source == nil {
		return ""
	}
	return m.source.ID
}

// AddClient registers a client for broadcast. Thread-safe.
func (m *MountPoint) AddClient(c *client.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.clientsByID[c.ID] = c
	m.rebuildSnapshotLocked()
	m.Stats.ClientCount.Store(int64(len(m.clientsByID)))
	slog.Info("client connected", "mount", m.Name, "client", c.ID)
}

// RemoveClient removes a client by ID. Called from Client.writeLoop via defer.
func (m *MountPoint) RemoveClient(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.clientsByID[id]; !ok {
		return
	}
	delete(m.clientsByID, id)
	m.rebuildSnapshotLocked()
	m.Stats.ClientCount.Store(int64(len(m.clientsByID)))
	slog.Info("client removed", "mount", m.Name, "client", id)
}

// Broadcast sends pkt to all connected clients using the atomic snapshot.
// Slow clients that cannot keep up are kicked via MarkSlowAndKick.
func (m *MountPoint) Broadcast(pkt *rtcm.RTCMPacket) {
	clients, _ := m.snapshot.Load().([]*client.Client)
	for _, c := range clients {
		select {
		case c.WriteChan <- pkt:
		case <-c.Done:
		default:
			m.Stats.SlowClients.Add(1)
			m.Stats.KickCount.Add(1)
			c.MarkSlowAndKick()
		}
	}
}

// DisconnectAll disconnects the source and all clients. Used during
// graceful shutdown or mountpoint deletion.
func (m *MountPoint) DisconnectAll() {
	m.mu.Lock()
	src := m.source
	m.source = nil
	m.Stats.SourceOnline.Store(0)

	clients := make([]*client.Client, 0, len(m.clientsByID))
	for _, c := range m.clientsByID {
		clients = append(clients, c)
	}
	m.mu.Unlock()

	if src != nil && src.Stop != nil {
		src.Stop()
	}

	for _, c := range clients {
		c.KickSlowConsumer()
	}
}

// ClientCount returns the number of currently connected clients.
func (m *MountPoint) ClientCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.clientsByID)
}

// ClientIDs returns a snapshot of currently connected client IDs.
func (m *MountPoint) ClientIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.clientsByID))
	for id := range m.clientsByID {
		ids = append(ids, id)
	}
	return ids
}

// rebuildSnapshotLocked must be called with mu held.
func (m *MountPoint) rebuildSnapshotLocked() {
	next := make([]*client.Client, 0, len(m.clientsByID))
	for _, c := range m.clientsByID {
		next = append(next, c)
	}
	m.snapshot.Store(next)
}
