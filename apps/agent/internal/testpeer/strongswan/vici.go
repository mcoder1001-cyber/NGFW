package strongswan

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"iter"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/strongswan/govici/vici"
)

// VICI access. The renderer talks to charon only through this socket (govici, MIT licence —
// structured messages instead of parsing swanctl's text output). Hardening around govici
// v0.8.2 (RF-1 review: unbounded reads):
//
//   - govici allocates whatever length a packet header announces; boundedConn fails the
//     session instead when a packet exceeds MaxPacket.
//   - govici's streaming (list-sas, list-conns) drops events silently once its 128-packet
//     buffer is full, so the renderer never streams an unbounded listing: it asks per
//     connection name (`ike` filter) with at most MaxSAsPerConn events per stream and
//     cross-checks the SA total against `stats`.
//   - CallStreaming releases the session lock before iterating, so a session is never shared:
//     every operation dials its own and closes it.

// MaxPacket bounds one VICI packet (charon's own request limit is 512 KiB).
const MaxPacket = 1 << 20

// Bounds of listings.
const (
	// MaxConns bounds the connections the renderer lists or loads.
	MaxConns = 4096
	// MaxSAsPerConn bounds the IKE_SAs listed per connection name.
	MaxSAsPerConn = 64
)

// ErrTooLarge is returned when charon sends more than the bounds allow.
var ErrTooLarge = errors.New("strongswan: VICI response exceeds bounds")

// ViciConn is the VICI client surface the renderer uses; *vici.Session implements it.
type ViciConn interface {
	Call(ctx context.Context, cmd string, in *vici.Message) (*vici.Message, error)
	CallStreaming(ctx context.Context, cmd, event string, in *vici.Message) iter.Seq2[*vici.Message, error]
	Subscribe(events ...string) error
	NotifyEvents(c chan<- vici.Event)
	Close() error
}

var _ ViciConn = (*vici.Session)(nil)

// Dialer opens a VICI session to the socket at path.
type Dialer func(ctx context.Context, socket string) (ViciConn, error)

// DialVICI dials charon's unix socket with a per-packet size bound.
func DialVICI(ctx context.Context, socket string) (ViciConn, error) {
	dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	s, err := vici.NewSession(vici.WithSocketPath(socket), vici.WithDialContext(
		func(_ context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			c, err := d.DialContext(dctx, network, addr)
			if err != nil {
				return nil, err
			}
			return &boundedConn{Conn: c, max: MaxPacket}, nil
		}))
	if err != nil {
		return nil, fmt.Errorf("%w: connect to VICI socket %s: %v", ErrDaemon, socket, err)
	}
	return s, nil
}

// boundedConn tracks VICI framing on reads (4-byte big-endian length, then the packet) and
// fails with ErrTooLarge instead of letting the reader allocate an oversized packet.
type boundedConn struct {
	net.Conn
	max       uint32
	mu        sync.Mutex
	hdr       [4]byte
	hdrN      int
	remaining uint32
}

func (b *boundedConn) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	if b.remaining == 0 {
		want := 4 - b.hdrN
		if len(p) > want {
			p = p[:want]
		}
		n, err := b.Conn.Read(p)
		copy(b.hdr[b.hdrN:], p[:n])
		b.hdrN += n
		if b.hdrN == 4 {
			b.hdrN = 0
			l := binary.BigEndian.Uint32(b.hdr[:])
			if l > b.max {
				_ = b.Close()
				return 0, fmt.Errorf("%w: packet of %d bytes (max %d)", ErrTooLarge, l, b.max)
			}
			b.remaining = l
		}
		return n, err
	}
	if uint32(len(p)) > b.remaining { //nolint:gosec // len(p) is non-negative
		p = p[:b.remaining]
	}
	n, err := b.Conn.Read(p)
	b.remaining -= uint32(n) //nolint:gosec // n <= len(p) <= remaining
	return n, err
}

// session is one VICI conversation with redacted errors and per-call timeouts.
type session struct {
	c       ViciConn
	secrets secretSet
}

func (r *Renderer) open(ctx context.Context) (*session, error) {
	c, err := r.dial(ctx, r.paths.ViciSocket)
	if err != nil {
		return nil, r.secrets.redactErr(fmt.Errorf("%w: %v", ErrDaemon, err))
	}
	return &session{c: c, secrets: r.secrets}, nil
}

func (s *session) Close() { _ = s.c.Close() }

// call runs one command; a charon-side failure (success=no) is an ErrDaemon error with the
// redacted errmsg.
func (s *session) call(ctx context.Context, cmd string, in *vici.Message) (*vici.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	out, err := s.c.Call(ctx, cmd, in)
	if err != nil {
		return out, &redactedError{msg: fmt.Sprintf("%v: vici %s: %s", ErrDaemon, cmd, s.secrets.redact(err.Error())), err: ErrDaemon}
	}
	return out, nil
}

// stream runs a streaming command and hands every event to fn; more than max events is an
// ErrTooLarge error (the listing would be incomplete).
func (s *session) stream(ctx context.Context, cmd, event string, in *vici.Message, maxEvents int, fn func(*vici.Message) error) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	n := 0
	for m, err := range s.c.CallStreaming(ctx, cmd, event, in) {
		if err != nil {
			return &redactedError{msg: fmt.Sprintf("%v: vici %s: %s", ErrDaemon, cmd, s.secrets.redact(err.Error())), err: ErrDaemon}
		}
		if n++; n > maxEvents {
			return fmt.Errorf("%w: %s streamed more than %d %s events", ErrTooLarge, cmd, maxEvents, event)
		}
		if err := fn(m); err != nil {
			return err
		}
	}
	return nil
}

// msg builds a message from ordered key/value pairs (values: string, []string, *Message).
func msg(kv ...any) *vici.Message {
	m := vici.NewMessage()
	for i := 0; i+1 < len(kv); i += 2 {
		_ = m.Set(kv[i].(string), kv[i+1]) //nolint:forcetypeassert // internal helper, keys are literals
	}
	return m
}

// str returns the string value of key ("" when absent or not a string).
func str(m *vici.Message, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m.Get(key).(string)
	return s
}

// strs returns the list value of key.
func strs(m *vici.Message, key string) []string {
	if m == nil {
		return nil
	}
	l, _ := m.Get(key).([]string)
	return l
}

// sub returns the section value of key.
func sub(m *vici.Message, key string) *vici.Message {
	if m == nil {
		return nil
	}
	s, _ := m.Get(key).(*vici.Message)
	return s
}

// ---------------------------------------------------------------------- commands used by several parts

// getConns returns the names of the connections loaded over VICI.
func (s *session) getConns(ctx context.Context) ([]string, error) {
	out, err := s.call(ctx, "get-conns", nil)
	if err != nil {
		return nil, err
	}
	names := strs(out, "conns")
	if len(names) > MaxConns {
		return nil, fmt.Errorf("%w: %d connections loaded (max %d)", ErrTooLarge, len(names), MaxConns)
	}
	return names, nil
}

// getShared returns the unique ids of the shared secrets loaded over VICI (never their data).
func (s *session) getShared(ctx context.Context) ([]string, error) {
	out, err := s.call(ctx, "get-shared", nil)
	if err != nil {
		return nil, err
	}
	return strs(out, "keys"), nil
}

// getPools returns the loaded pool names.
func (s *session) getPools(ctx context.Context) ([]string, error) {
	out, err := s.call(ctx, "get-pools", nil)
	if err != nil {
		return nil, err
	}
	return out.Keys(), nil
}

// getAuthorities returns the loaded authority names.
func (s *session) getAuthorities(ctx context.Context) ([]string, error) {
	out, err := s.call(ctx, "get-authorities", nil)
	if err != nil {
		return nil, err
	}
	return strs(out, "authorities"), nil
}

// listConn returns the list-conns entry of one connection (nil when not loaded).
func (s *session) listConn(ctx context.Context, name string) (*vici.Message, error) {
	var found *vici.Message
	err := s.stream(ctx, "list-conns", "list-conn", msg("ike", name), 1, func(m *vici.Message) error {
		found = sub(m, name)
		return nil
	})
	return found, err
}

// listSAs returns the IKE_SAs of one connection name, at most MaxSAsPerConn; truncated
// reports that there were more (review L3: callers degrade instead of failing).
func (s *session) listSAs(ctx context.Context, name string) ([]*vici.Message, bool, error) {
	var out []*vici.Message
	err := s.stream(ctx, "list-sas", "list-sa", msg("ike", name, "noblock", "yes"), MaxSAsPerConn, func(m *vici.Message) error {
		for _, k := range m.Keys() {
			if sa := sub(m, k); sa != nil && k == name {
				out = append(out, sa)
			}
		}
		return nil
	})
	if errors.Is(err, ErrTooLarge) {
		return out, true, nil
	}
	return out, false, err
}

// terminateTimeout bounds a graceful terminate (DELETE exchange with the peer).
const terminateTimeout = "3000"

// terminateIKE deletes one IKE_SA (and its CHILD_SAs) by unique id: gracefully first, so the
// peer drops its side too, forced when the peer does not answer. A SA that is already gone
// is not an error.
func (s *session) terminateIKE(ctx context.Context, id string) error {
	return s.terminate(ctx, "ike-id", id)
}

// terminateChild deletes one CHILD_SA by unique id (the IKE_SA stays).
func (s *session) terminateChild(ctx context.Context, id string) error {
	return s.terminate(ctx, "child-id", id)
}

func (s *session) terminate(ctx context.Context, key, id string) error {
	if !uintRe.MatchString(id) {
		return fmt.Errorf("%w: terminate %s %q", ErrDaemon, key, id)
	}
	_, err := s.call(ctx, "terminate", msg(key, id, "timeout", terminateTimeout))
	if err == nil || strings.Contains(err.Error(), "no matching") {
		return nil
	}
	if key == "child-id" {
		return err
	}
	_, ferr := s.call(ctx, "terminate", msg(key, id, "force", "yes", "timeout", "1000"))
	if ferr == nil || strings.Contains(ferr.Error(), "no matching") {
		return nil
	}
	return errors.Join(err, ferr)
}

// initiateAsync starts a CHILD_SA without waiting for the result (a peer that is down must
// not fail a commit; the SA check in Apply only rejects contradicting SAs).
func (s *session) initiateAsync(ctx context.Context, conn, child string) error {
	_, err := s.call(ctx, "initiate", msg("child", child, "ike", conn, "timeout", "-1", "init-limits", "no"))
	if err != nil && !strings.Contains(err.Error(), "establishing") {
		return fmt.Errorf("initiate %s/%s: %w", conn, child, err)
	}
	return nil
}
