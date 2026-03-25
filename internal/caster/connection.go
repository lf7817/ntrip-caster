package caster

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"ntrip-caster/internal/account"
	"ntrip-caster/internal/auth"
	"ntrip-caster/internal/client"
	"ntrip-caster/internal/config"
	"ntrip-caster/internal/mountpoint"
	"ntrip-caster/internal/source"
	"ntrip-caster/internal/sourcetable"

	"github.com/google/uuid"
)

// connHandler encapsulates the dependencies needed to handle a single
// NTRIP TCP connection.
type connHandler struct {
	cfg     *config.Config
	mgr     *mountpoint.Manager
	acctSvc *account.Service

	wg *sync.WaitGroup
}

func (h *connHandler) handle(conn net.Conn) {
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(30 * time.Second)
	}

	reader := bufio.NewReaderSize(conn, 4096)
	req, err := ParseRequest(reader)
	if err != nil {
		slog.Debug("parse request failed", "remote", conn.RemoteAddr(), "err", err)
		conn.Close()
		return
	}

	switch req.Type {
	case RequestSourcetable:
		h.handleSourcetable(conn)
	case RequestRover:
		h.handleRover(conn, req)
	case RequestSourceRev1:
		h.handleSourceRev1(conn, req)
	case RequestSourceRev2:
		h.handleSourceRev2(conn, req)
	default:
		conn.Close()
	}
}

// --- Sourcetable ---

func (h *connHandler) handleSourcetable(conn net.Conn) {
	defer conn.Close()
	body := sourcetable.Generate(h.mgr)
	_, _ = conn.Write([]byte(body))
}

// --- Rover ---

func (h *connHandler) handleRover(conn net.Conn, req *NTRIPRequest) {
	// Authenticate
	if h.cfg.Auth.Enabled && h.cfg.Auth.NtripRoverAuth == "basic" {
		user, err := h.authenticateBasic(req, account.RoleRover)
		if err != nil || user == nil {
			writeResponse(conn, "HTTP/1.1 401 Unauthorized\r\n\r\n")
			conn.Close()
			return
		}

		// Check mountpoint binding (admin users skip this check)
		if user.Role != account.RoleAdmin {
			has, err := h.acctSvc.HasBinding(user.ID, req.MountPoint)
			if err != nil || !has {
				writeResponse(conn, "HTTP/1.1 403 Forbidden\r\n\r\n")
				conn.Close()
				return
			}
		}
	}

	// Lookup mountpoint
	mp := h.mgr.Get(req.MountPoint)
	if mp == nil || !mp.Enabled {
		writeResponse(conn, "HTTP/1.1 404 Not Found\r\n\r\n")
		conn.Close()
		return
	}

	// Reject if no source is online
	if !mp.HasSource() {
		writeResponse(conn, "HTTP/1.1 503 Service Unavailable\r\n\r\n")
		conn.Close()
		return
	}

	// Respond
	writeResponse(conn, "ICY 200 OK\r\n\r\n")

	// Register client
	id := uuid.New().String()
	c := client.New(
		id, conn, mp, mp.Name,
		mp.WriteQueue,
		mp.WriteTimeout,
	)
	mp.AddClient(c)

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		c.WriteLoop()
	}()
}

// --- Source Rev1 ---

func (h *connHandler) handleSourceRev1(conn net.Conn, req *NTRIPRequest) {
	if h.cfg.Auth.Enabled {
		ok, err := h.authenticateSource(req)
		if err != nil || !ok {
			writeResponse(conn, "ERROR - Bad Password\r\n")
			conn.Close()
			return
		}
	}

	mp := h.mgr.Get(req.MountPoint)
	if mp == nil || !mp.Enabled {
		writeResponse(conn, "HTTP/1.1 404 Not Found\r\n\r\n")
		conn.Close()
		return
	}

	id := uuid.New().String()
	src := source.New(id, conn, mp)
	if src == nil {
		writeResponse(conn, "HTTP/1.1 409 Conflict\r\n\r\n")
		conn.Close()
		return
	}

	writeResponse(conn, "ICY 200 OK\r\n\r\n")

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		src.ReadLoop()
	}()
}

// --- Source Rev2 ---

func (h *connHandler) handleSourceRev2(conn net.Conn, req *NTRIPRequest) {
	if h.cfg.Auth.Enabled {
		user, err := h.authenticateBasic(req, account.RoleBase)
		if err != nil || user == nil {
			writeResponse(conn, "HTTP/1.1 401 Unauthorized\r\n\r\n")
			conn.Close()
			return
		}

		if h.cfg.Auth.NtripSourceAuth == "user_binding" {
			has, err := h.acctSvc.HasBinding(user.ID, req.MountPoint)
			if err != nil || !has {
				writeResponse(conn, "HTTP/1.1 403 Forbidden\r\n\r\n")
				conn.Close()
				return
			}
		}
	}

	mp := h.mgr.Get(req.MountPoint)
	if mp == nil || !mp.Enabled {
		writeResponse(conn, "HTTP/1.1 404 Not Found\r\n\r\n")
		conn.Close()
		return
	}

	id := uuid.New().String()
	src := source.New(id, conn, mp)
	if src == nil {
		writeResponse(conn, "HTTP/1.1 409 Conflict\r\n\r\n")
		conn.Close()
		return
	}

	writeResponse(conn, "HTTP/1.1 200 OK\r\n\r\n")

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		src.ReadLoop()
	}()
}

// --- helpers ---

func (h *connHandler) authenticateBasic(req *NTRIPRequest, role string) (*account.User, error) {
	authHeader := req.Headers["Authorization"]
	if authHeader == "" {
		return nil, nil
	}
	username, password, ok := auth.ParseBasicAuth(authHeader)
	if !ok {
		return nil, nil
	}
	user, err := h.acctSvc.Authenticate(username, password)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}
	// admin users can do anything
	if user.Role == account.RoleAdmin {
		return user, nil
	}
	if user.Role != role {
		return nil, nil
	}
	return user, nil
}

func (h *connHandler) authenticateSource(req *NTRIPRequest) (bool, error) {
	if h.cfg.Auth.NtripSourceAuth == "user_binding" {
		authHeader := req.Headers["Authorization"]
		if authHeader != "" {
			// If the device provided Authorization: Basic, prefer user_binding auth.
			user, err := h.authenticateBasic(req, account.RoleBase)
			if err != nil || user == nil {
				return false, err
			}
			has, err := h.acctSvc.HasBinding(user.ID, req.MountPoint)
			return has, err
		}

		// Rev1 SOURCE always carries a password field. For legacy devices that do
		// not send Authorization, fall back to per-mountpoint secret validation.
		ok, err := h.acctSvc.VerifyMountPointSourceSecret(req.MountPoint, req.Password)
		return ok, err
	}
	return true, nil
}

func writeResponse(conn net.Conn, resp string) {
	_, err := fmt.Fprint(conn, resp)
	if err != nil {
		slog.Debug("write response failed", "remote", conn.RemoteAddr(), "err", err)
	}
}
