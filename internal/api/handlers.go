package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"ntrip-caster/internal/account"
	"ntrip-caster/internal/config"
	"ntrip-caster/internal/mountpoint"
)

// Handlers holds all admin API handlers and their dependencies.
type Handlers struct {
	cfg     *config.Config
	acctSvc *account.Service
	mgr     *mountpoint.Manager
	sess    *SessionManager
}

// NewHandlers creates a Handlers instance.
func NewHandlers(cfg *config.Config, acctSvc *account.Service, mgr *mountpoint.Manager, sess *SessionManager) *Handlers {
	return &Handlers{cfg: cfg, acctSvc: acctSvc, mgr: mgr, sess: sess}
}

// --- Login / Logout ---

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.acctSvc.Authenticate(req.Username, req.Password)
	if err != nil {
		slog.Error("login auth error", "err", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if user == nil || user.Role != account.RoleAdmin {
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := h.sess.Create(user.Username)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	jsonOK(w, map[string]string{"status": "ok", "username": user.Username})
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		h.sess.Destroy(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:   sessionCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- Users ---

func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.acctSvc.ListUsers()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, users)
}

func (h *Handlers) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Username == "" || req.Password == "" || req.Role == "" {
		jsonError(w, "username, password, and role are required", http.StatusBadRequest)
		return
	}

	user, err := h.acctSvc.CreateUser(req.Username, req.Password, req.Role)
	if err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}
	jsonOK(w, user)
}

func (h *Handlers) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		jsonError(w, "invalid user id", http.StatusBadRequest)
		return
	}

	var req struct {
		Role     string `json:"role"`
		Enabled  *bool  `json:"enabled"`
		Password string `json:"password,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.acctSvc.GetUser(id)
	if err != nil || user == nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	role := user.Role
	if req.Role != "" {
		role = req.Role
	}
	enabled := user.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if err := h.acctSvc.UpdateUser(id, role, enabled); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if req.Password != "" {
		if err := h.acctSvc.UpdatePassword(id, req.Password); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Disconnect all connections when user is disabled
	if user.Enabled && !enabled {
		h.mgr.DisconnectUser(id)
	}

	jsonOK(w, map[string]string{"status": "ok"})
}

func (h *Handlers) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		jsonError(w, "invalid user id", http.StatusBadRequest)
		return
	}

	// Disconnect all connections before deleting
	h.mgr.DisconnectUser(id)

	if err := h.acctSvc.DeleteUser(id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- Mountpoints ---

func (h *Handlers) ListMountpoints(w http.ResponseWriter, r *http.Request) {
	rows, err := h.acctSvc.ListMountPointRows()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type mpInfo struct {
		account.MountPointRow
		SourceOnline bool  `json:"source_online"`
		ClientCount  int64 `json:"client_count"`
	}

	result := make([]mpInfo, 0, len(rows))
	for _, row := range rows {
		info := mpInfo{MountPointRow: *row}
		if mp := h.mgr.Get(row.Name); mp != nil {
			snap := mp.Stats.Snapshot()
			info.SourceOnline = snap.SourceOnline
			info.ClientCount = snap.ClientCount
		}
		result = append(result, info)
	}
	jsonOK(w, result)
}

func (h *Handlers) CreateMountpoint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		Format       string `json:"format"`
		SourceSecret string `json:"source_secret,omitempty"`
		MaxClients   int    `json:"max_clients"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.Format == "" {
		req.Format = "RTCM3"
	}

	row, err := h.acctSvc.CreateMountPointRow(req.Name, req.Description, req.Format)
	if err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}

	if req.SourceSecret != "" {
		if err := h.acctSvc.SetMountPointSourceSecret(row.ID, req.SourceSecret); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if req.MaxClients > 0 {
		if err := h.acctSvc.SetMountPointMaxClients(row.ID, req.MaxClients); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	wq := h.cfg.MountpointDefaults.WriteQueue
	wt := h.cfg.MountpointDefaults.WriteTimeout
	_, _ = h.mgr.Create(req.Name, req.Description, req.Format, wq, wt, req.MaxClients)

	jsonOK(w, row)
}

func (h *Handlers) UpdateMountpoint(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		jsonError(w, "invalid mountpoint id", http.StatusBadRequest)
		return
	}

	var req struct {
		Description  string  `json:"description"`
		Format       string  `json:"format"`
		Enabled      *bool   `json:"enabled"`
		SourceSecret *string `json:"source_secret,omitempty"` // nil: unchanged, "": clear, other: set
		MaxClients   *int    `json:"max_clients,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	row, err := h.acctSvc.GetMountPointRowByID(id)
	if err != nil || row == nil {
		jsonError(w, "mountpoint not found", http.StatusNotFound)
		return
	}

	desc := row.Description
	if req.Description != "" {
		desc = req.Description
	}
	format := row.Format
	if req.Format != "" {
		format = req.Format
	}
	enabled := row.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	maxClients := row.MaxClients
	if req.MaxClients != nil {
		maxClients = *req.MaxClients
	}

	if err := h.acctSvc.UpdateMountPointRow(id, desc, format, enabled); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if req.MaxClients != nil {
		if err := h.acctSvc.SetMountPointMaxClients(id, *req.MaxClients); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if req.SourceSecret != nil {
		if err := h.acctSvc.SetMountPointSourceSecret(id, *req.SourceSecret); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	wq := h.cfg.MountpointDefaults.WriteQueue
	wt := h.cfg.MountpointDefaults.WriteTimeout
	_ = h.mgr.UpdateMountPoint(row.Name, desc, format, enabled, wq, wt, maxClients)

	jsonOK(w, map[string]string{"status": "ok"})
}

func (h *Handlers) DeleteMountpoint(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		jsonError(w, "invalid mountpoint id", http.StatusBadRequest)
		return
	}

	row, err := h.acctSvc.GetMountPointRowByID(id)
	if err != nil || row == nil {
		jsonError(w, "mountpoint not found", http.StatusNotFound)
		return
	}

	_ = h.mgr.Delete(row.Name)

	if err := h.acctSvc.DeleteMountPointRow(id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- Online Sources / Clients ---

func (h *Handlers) ListSources(w http.ResponseWriter, r *http.Request) {
	mountFilter := r.URL.Query().Get("mountpoint")
	usernameFilter := r.URL.Query().Get("username")

	type srcInfo struct {
		MountPoint  string `json:"mountpoint"`
		SourceID    string `json:"source_id"`
		UserID      int64  `json:"user_id"`
		Username    string `json:"username"`
		BytesIn     int64  `json:"bytes_in"`
		ConnectedAt string `json:"connected_at"`
		Duration    int64  `json:"duration_seconds"`
	}
	var result []srcInfo
	for _, mp := range h.mgr.List() {
		si := mp.GetSourceInfo()
		if si == nil || si.ID == "" {
			continue
		}

		// Filter by mountpoint
		if mountFilter != "" && mp.Name != mountFilter {
			continue
		}

		userID := si.UserID
		var username string
		if userID > 0 {
			user, err := h.acctSvc.GetUser(userID)
			if err == nil && user != nil {
				username = user.Username
			}
		}

		// Filter by username
		if usernameFilter != "" && username != usernameFilter {
			continue
		}

		var bytesIn int64
		if si.BytesIn != nil {
			bytesIn = si.BytesIn.Load()
		}
		result = append(result, srcInfo{
			MountPoint:  mp.Name,
			SourceID:    si.ID,
			UserID:      userID,
			Username:    username,
			BytesIn:     bytesIn,
			ConnectedAt: si.StartTime.Format(time.RFC3339),
			Duration:    int64(time.Since(si.StartTime).Seconds()),
		})
	}
	jsonOK(w, result)
}

func (h *Handlers) ListClients(w http.ResponseWriter, r *http.Request) {
	mountFilter := r.URL.Query().Get("mountpoint")
	usernameFilter := r.URL.Query().Get("username")

	type cliInfo struct {
		MountPoint  string `json:"mountpoint"`
		ClientID    string `json:"client_id"`
		UserID      int64  `json:"user_id"`
		Username    string `json:"username"`
		BytesOut    int64  `json:"bytes_out"`
		ConnectedAt string `json:"connected_at"`
		Duration    int64  `json:"duration_seconds"`
	}

	var result []cliInfo
	for _, mp := range h.mgr.List() {
		if mountFilter != "" && mp.Name != mountFilter {
			continue
		}

		for _, c := range mp.ClientInfos() {
			var username string
			if c.UserID > 0 {
				user, err := h.acctSvc.GetUser(c.UserID)
				if err == nil && user != nil {
					username = user.Username
				}
			}

			if usernameFilter != "" && username != usernameFilter {
				continue
			}

			result = append(result, cliInfo{
				MountPoint:  mp.Name,
				ClientID:    c.ID,
				UserID:      c.UserID,
				Username:    username,
				BytesOut:    c.BytesOut,
				ConnectedAt: c.ConnectedAt.Format(time.RFC3339),
				Duration:    int64(time.Since(c.ConnectedAt).Seconds()),
			})
		}
	}

	jsonOK(w, result)
}

func (h *Handlers) KickSource(w http.ResponseWriter, r *http.Request) {
	mountName := r.PathValue("mount")
	if mountName == "" {
		jsonError(w, "mountpoint name required", http.StatusBadRequest)
		return
	}
	mp := h.mgr.Get(mountName)
	if mp == nil {
		jsonError(w, "mountpoint not found", http.StatusNotFound)
		return
	}
	mp.ClearSource(mp.SourceID())
	jsonOK(w, map[string]string{"status": "ok"})
}

func (h *Handlers) KickClient(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	if clientID == "" {
		jsonError(w, "client id required", http.StatusBadRequest)
		return
	}
	for _, mp := range h.mgr.List() {
		mp.KickClient(clientID)
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- Stats ---

func (h *Handlers) Stats(w http.ResponseWriter, r *http.Request) {
	type mountStat struct {
		Name        string `json:"name"`
		ClientCount int64  `json:"client_count"`
		SourceOnline bool  `json:"source_online"`
		BytesIn     int64  `json:"bytes_in"`
		BytesOut    int64  `json:"bytes_out"`
		SlowClients int64  `json:"slow_clients"`
		KickCount   int64  `json:"kick_count"`
	}

	var totalClients, totalSources int64
	var mounts []mountStat

	for _, mp := range h.mgr.List() {
		snap := mp.Stats.Snapshot()
		mounts = append(mounts, mountStat{
			Name:         mp.Name,
			ClientCount:  snap.ClientCount,
			SourceOnline: snap.SourceOnline,
			BytesIn:      snap.BytesIn,
			BytesOut:     snap.BytesOut,
			SlowClients:  snap.SlowClients,
			KickCount:    snap.KickCount,
		})
		totalClients += snap.ClientCount
		if snap.SourceOnline {
			totalSources++
		}
	}

	result := map[string]any{
		"total_clients": totalClients,
		"total_sources": totalSources,
		"mountpoints":   mounts,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}
	jsonOK(w, result)
}

// --- Bindings ---

func (h *Handlers) ListBindings(w http.ResponseWriter, r *http.Request) {
	bindings, err := h.acctSvc.ListBindings()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, bindings)
}

func (h *Handlers) ListUserBindings(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		jsonError(w, "invalid user id", http.StatusBadRequest)
		return
	}
	bindings, err := h.acctSvc.ListBindingsByUser(id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, bindings)
}

func (h *Handlers) CreateBinding(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID       int64 `json:"user_id"`
		MountPointID int64 `json:"mountpoint_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.UserID == 0 || req.MountPointID == 0 {
		jsonError(w, "user_id and mountpoint_id are required", http.StatusBadRequest)
		return
	}

	if err := h.acctSvc.AddBinding(req.UserID, req.MountPointID); err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (h *Handlers) DeleteBinding(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r.PathValue("id"))
	if !ok {
		jsonError(w, "invalid binding id", http.StatusBadRequest)
		return
	}
	if err := h.acctSvc.RemoveBinding(id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- helpers ---

func jsonOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func parseID(s string) (int64, bool) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
