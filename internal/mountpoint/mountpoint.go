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
	enabled      atomic.Bool // thread-safe enabled state
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
	ID     string
	UserID int64 // user who established this connection (0 if no auth)
	Done   chan struct{}
	Stop   func() // called to close the source connection
}

// NewMountPoint creates an enabled mountpoint with the given defaults.
func NewMountPoint(name, description, format string, writeQueue int, writeTimeout time.Duration) *MountPoint {
	mp := &MountPoint{
		Name:         name,
		Description:  description,
		Format:       format,
		WriteQueue:   writeQueue,
		WriteTimeout: writeTimeout,
		clientsByID:  make(map[string]*client.Client),
	}
	mp.enabled.Store(true)
	mp.snapshot.Store(([]*client.Client)(nil))
	return mp
}

// IsEnabled returns the enabled state. Thread-safe.
func (m *MountPoint) IsEnabled() bool {
	return m.enabled.Load()
}

// SetEnabled sets the enabled state. Thread-safe.
func (m *MountPoint) SetEnabled(enabled bool) {
	m.enabled.Store(enabled)
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

// ClearSource removes the current source, closes its connection, and disconnects
// all rover clients. It is a no-op if src does not match the current source ID
// (prevents a stale goroutine from clearing a new source).
func (m *MountPoint) ClearSource(srcID string) {
	m.mu.Lock()
	if m.source == nil || m.source.ID != srcID {
		m.mu.Unlock()
		return
	}
	src := m.source
	m.source = nil
	m.Stats.SourceOnline.Store(0)
	slog.Info("source disconnected", "mount", m.Name, "source", srcID)

	// Disconnect all rover clients since they can no longer receive data
	clients := make([]*client.Client, 0, len(m.clientsByID))
	for _, c := range m.clientsByID {
		clients = append(clients, c)
	}
	m.mu.Unlock()

	// Close the source TCP connection
	if src.Stop != nil {
		src.Stop()
	}

	for _, c := range clients {
		c.KickSlowConsumer()
	}
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

// SourceUserID returns the user ID of the current source, or 0 if none or no user binding.
func (m *MountPoint) SourceUserID() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.source == nil {
		return 0
	}
	return m.source.UserID
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
// This only removes the client from the registry; use KickClient to close the connection.
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

// KickClient removes a client and closes its TCP connection.
func (m *MountPoint) KickClient(id string) {
	m.mu.Lock()
	c, ok := m.clientsByID[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.clientsByID, id)
	m.rebuildSnapshotLocked()
	m.Stats.ClientCount.Store(int64(len(m.clientsByID)))
	m.mu.Unlock()

	slog.Info("client kicked", "mount", m.Name, "client", id)
	c.KickSlowConsumer()
}

// KickClientsByUser disconnects all clients belonging to a specific user.
// Returns true if any clients were disconnected.
func (m *MountPoint) KickClientsByUser(userID int64) bool {
	m.mu.Lock()
	var toKick []*client.Client
	for id, c := range m.clientsByID {
		if c.UserID == userID {
			toKick = append(toKick, c)
			delete(m.clientsByID, id)
		}
	}
	if len(toKick) > 0 {
		m.rebuildSnapshotLocked()
		m.Stats.ClientCount.Store(int64(len(m.clientsByID)))
	}
	m.mu.Unlock()

	for _, c := range toKick {
		slog.Info("client kicked (user disabled)", "mount", m.Name, "client", c.ID, "user", userID)
		c.KickSlowConsumer()
	}
	return len(toKick) > 0
}

// KickSourceByUser disconnects the source and all its rover clients
// if the source belongs to the specified user.
// Returns true if the source was disconnected.
func (m *MountPoint) KickSourceByUser(userID int64) bool {
	m.mu.Lock()
	if m.source == nil || m.source.UserID != userID {
		m.mu.Unlock()
		return false
	}
	src := m.source
	m.source = nil
	m.Stats.SourceOnline.Store(0)

	// Also collect rover clients to disconnect
	clients := make([]*client.Client, 0, len(m.clientsByID))
	for _, c := range m.clientsByID {
		clients = append(clients, c)
	}
	// Clear all clients since source is gone
	for id := range m.clientsByID {
		delete(m.clientsByID, id)
	}
	m.rebuildSnapshotLocked()
	m.Stats.ClientCount.Store(0)
	m.mu.Unlock()

	slog.Info("source kicked (user disabled)", "mount", m.Name, "source", src.ID, "user", userID)
	if src.Stop != nil {
		src.Stop()
	}

	for _, c := range clients {
		c.KickSlowConsumer()
	}
	return true
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

// ClientInfo contains basic information about a connected client.
type ClientInfo struct {
	ID     string
	UserID int64
}

// ClientInfos returns a snapshot of currently connected clients with their user IDs.
func (m *MountPoint) ClientInfos() []ClientInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	infos := make([]ClientInfo, 0, len(m.clientsByID))
	for _, c := range m.clientsByID {
		infos = append(infos, ClientInfo{ID: c.ID, UserID: c.UserID})
	}
	return infos
}

// rebuildSnapshotLocked must be called with mu held.
func (m *MountPoint) rebuildSnapshotLocked() {
	next := make([]*client.Client, 0, len(m.clientsByID))
	for _, c := range m.clientsByID {
		next = append(next, c)
	}
	m.snapshot.Store(next)
}
