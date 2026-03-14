// Package api provides the Admin HTTP API server and handlers.
package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

// SessionManager provides cookie-based session management for the admin API.
type SessionManager struct {
	mu       sync.Mutex
	sessions map[string]*sessionEntry
}

type sessionEntry struct {
	username  string
	expiresAt time.Time
}

const (
	sessionCookieName = "ntrip_session"
	sessionTTL        = 24 * time.Hour
)

// NewSessionManager creates a new in-memory session store.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*sessionEntry),
	}
}

// Create generates a new session token for the given username and returns it.
func (sm *SessionManager) Create(username string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)

	sm.mu.Lock()
	sm.sessions[token] = &sessionEntry{
		username:  username,
		expiresAt: time.Now().Add(sessionTTL),
	}
	sm.mu.Unlock()
	return token, nil
}

// Validate checks if a token is valid and returns the username.
func (sm *SessionManager) Validate(token string) (string, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	entry, ok := sm.sessions[token]
	if !ok {
		return "", false
	}
	if time.Now().After(entry.expiresAt) {
		delete(sm.sessions, token)
		return "", false
	}
	return entry.username, true
}

// Destroy removes a session.
func (sm *SessionManager) Destroy(token string) {
	sm.mu.Lock()
	delete(sm.sessions, token)
	sm.mu.Unlock()
}

// AuthMiddleware wraps an http.Handler and requires a valid admin session.
func (sm *SessionManager) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if _, ok := sm.Validate(cookie.Value); !ok {
			http.Error(w, `{"error":"session expired"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
