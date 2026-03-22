package caster

import (
	"context"
	"log/slog"
	"net"
	"sync"

	"ntrip-caster/internal/account"
	"ntrip-caster/internal/config"
	"ntrip-caster/internal/limiter"
	"ntrip-caster/internal/mountpoint"
)

// Server is the NTRIP TCP server that accepts Base Station and Rover
// connections on a single port.
type Server struct {
	cfg      *config.Config
	listener net.Listener
	mgr      *mountpoint.Manager
	acctSvc  *account.Service
	limiter  *limiter.IPLimiter

	wg   sync.WaitGroup
	done chan struct{}
}

// NewServer creates a new NTRIP TCP server.
func NewServer(cfg *config.Config, mgr *mountpoint.Manager, acctSvc *account.Service) *Server {
	return &Server{
		cfg:     cfg,
		mgr:     mgr,
		acctSvc: acctSvc,
		limiter: limiter.NewIPLimiter(cfg.Limits.MaxConnPerIP),
		done:    make(chan struct{}),
	}
}

// ListenAndServe starts listening on the configured address and enters the
// accept loop. It blocks until Shutdown is called or the listener is closed.
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.cfg.Server.Listen)
	if err != nil {
		return err
	}
	s.listener = ln
	slog.Info("NTRIP server listening", "addr", ln.Addr())

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.done:
				return nil
			default:
			}
			slog.Warn("accept error", "err", err)
			continue
		}

		ip := extractIP(conn.RemoteAddr())
		if !s.limiter.Allow(ip) {
			slog.Debug("connection rate limited", "ip", ip)
			conn.Close()
			continue
		}

		go func() {
			defer s.limiter.Release(ip)
			handler := &connHandler{
				cfg:     s.cfg,
				mgr:     s.mgr,
				acctSvc: s.acctSvc,
				wg:      &s.wg,
			}
			handler.handle(conn)
		}()
	}
}

// Shutdown gracefully stops the server. It closes the listener to stop
// accepting new connections, then disconnects all mountpoints and waits
// for goroutines to finish.
func (s *Server) Shutdown(ctx context.Context) error {
	close(s.done)
	if s.listener != nil {
		_ = s.listener.Close()
	}

	s.mgr.DisconnectAll()

	waitCh := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		slog.Info("all NTRIP goroutines exited")
		return nil
	case <-ctx.Done():
		slog.Warn("NTRIP shutdown timed out")
		return ctx.Err()
	}
}

// MountManager returns the underlying mountpoint manager.
func (s *Server) MountManager() *mountpoint.Manager {
	return s.mgr
}

func extractIP(addr net.Addr) string {
	if ta, ok := addr.(*net.TCPAddr); ok {
		return ta.IP.String()
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}
