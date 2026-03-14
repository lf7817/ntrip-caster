package mountpoint

import (
	"fmt"
	"sync"
	"time"
)

// Manager is a thread-safe registry of all mountpoints.
type Manager struct {
	mu     sync.RWMutex
	mounts map[string]*MountPoint
}

// NewManager creates an empty mountpoint manager.
func NewManager() *Manager {
	return &Manager{
		mounts: make(map[string]*MountPoint),
	}
}

// Get returns a mountpoint by name, or nil if not found.
func (mgr *Manager) Get(name string) *MountPoint {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	return mgr.mounts[name]
}

// Create creates a new mountpoint. Returns an error if one with the same
// name already exists.
func (mgr *Manager) Create(name, description, format string, writeQueue int, writeTimeout time.Duration) (*MountPoint, error) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if _, exists := mgr.mounts[name]; exists {
		return nil, fmt.Errorf("mountpoint %q already exists", name)
	}

	mp := NewMountPoint(name, description, format, writeQueue, writeTimeout)
	mgr.mounts[name] = mp
	return mp, nil
}

// Delete removes a mountpoint. It disconnects the source and all clients
// before removal. Returns an error if the mountpoint does not exist.
func (mgr *Manager) Delete(name string) error {
	mgr.mu.Lock()
	mp, exists := mgr.mounts[name]
	if !exists {
		mgr.mu.Unlock()
		return fmt.Errorf("mountpoint %q not found", name)
	}
	delete(mgr.mounts, name)
	mgr.mu.Unlock()

	mp.DisconnectAll()
	return nil
}

// List returns a snapshot of all mountpoints.
func (mgr *Manager) List() []*MountPoint {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()
	list := make([]*MountPoint, 0, len(mgr.mounts))
	for _, mp := range mgr.mounts {
		list = append(list, mp)
	}
	return list
}

// DisconnectAll disconnects every mountpoint's source and clients.
func (mgr *Manager) DisconnectAll() {
	mgr.mu.RLock()
	mounts := make([]*MountPoint, 0, len(mgr.mounts))
	for _, mp := range mgr.mounts {
		mounts = append(mounts, mp)
	}
	mgr.mu.RUnlock()

	for _, mp := range mounts {
		mp.DisconnectAll()
	}
}

// UpdateMountPoint updates metadata fields of an existing mountpoint.
func (mgr *Manager) UpdateMountPoint(name string, description string, format string, enabled bool, writeQueue int, writeTimeout time.Duration) error {
	mgr.mu.RLock()
	mp, exists := mgr.mounts[name]
	mgr.mu.RUnlock()
	if !exists {
		return fmt.Errorf("mountpoint %q not found", name)
	}

	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.Description = description
	mp.Format = format
	mp.Enabled = enabled
	mp.WriteQueue = writeQueue
	mp.WriteTimeout = writeTimeout
	return nil
}
