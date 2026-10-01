package promexport

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// Handler serves the metrics text at /metrics, gated by an optional CIDR allow-list. It is used by the
// external listener (management.prometheus); the loopback /metrics endpoint P05 built is unaffected.
type Handler struct {
	src    StatsSource
	prefix string
	allow  []*net.IPNet
}

// NewHandler builds a handler. allow is the parsed CIDR list ("" entries and bad CIDRs are the caller's to
// reject at validation); an empty list allows any source that reaches the listener.
func NewHandler(src StatsSource, prefix string, allow []*net.IPNet) *Handler {
	return &Handler{src: src, prefix: prefix, allow: allow}
}

// ParseAllow parses CIDR strings (a bare address becomes a /32 or /128).
func ParseAllow(cidrs []string) ([]*net.IPNet, error) {
	var out []*net.IPNet
	for _, c := range cidrs {
		if ip := net.ParseIP(c); ip != nil {
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			return nil, fmt.Errorf("promexport: bad allow entry %q: %w", c, err)
		}
		out = append(out, n)
	}
	return out, nil
}

func (h *Handler) allowed(remote string) bool {
	if len(h.allow) == 0 {
		return true
	}
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range h.allow {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/metrics" {
		http.NotFound(w, r)
		return
	}
	if !h.allowed(r.RemoteAddr) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var buf bytes.Buffer
	if err := Collect(ctx, h.src, h.prefix, &buf); err != nil {
		http.Error(w, "collect error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// Listener is the external Prometheus HTTP server (management.prometheus). Start/Stop are idempotent.
type Listener struct {
	mu      sync.Mutex
	srv     *http.Server
	ln      net.Listener
	addr    string
	handler *liveHandler
}

type liveHandler struct {
	mu sync.RWMutex
	h  http.Handler
}

func (h *liveHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	next := h.h
	h.mu.RUnlock()
	next.ServeHTTP(w, r)
}
func (h *liveHandler) set(next http.Handler) { h.mu.Lock(); h.h = next; h.mu.Unlock() }

// Start binds addr and serves h. A running listener on a different address is replaced.
func (l *Listener) Start(addr string, h http.Handler) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv != nil && l.addr == addr {
		l.handler.set(h)
		return nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("promexport: listen %s: %w", addr, err)
	}
	l.stopLocked()
	live := &liveHandler{h: h}
	srv := &http.Server{Handler: live, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second}
	l.handler = live
	l.srv, l.ln, l.addr = srv, ln, addr
	go func() { _ = srv.Serve(ln) }()
	return nil
}

// Addr is the bound address (useful when the port was 0 in a test).
func (l *Listener) Addr() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ln == nil {
		return ""
	}
	return l.ln.Addr().String()
}

// Stop shuts the listener down.
func (l *Listener) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopLocked()
}

func (l *Listener) stopLocked() {
	if l.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := l.srv.Shutdown(ctx); err != nil {
			// Shutdown leaves active requests alive after its deadline. Close cancels
			// their request contexts before the listener loses its server reference.
			_ = l.srv.Close()
		}
		l.srv, l.ln, l.addr = nil, nil, ""
	}
}
