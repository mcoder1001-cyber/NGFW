package snmpagent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// Defaults.
const (
	DefaultPriority   = 127
	openTimeoutSec    = 5
	responseTimeout   = 5 * time.Second
	pingInterval      = 15 * time.Second
	minBackoff        = 500 * time.Millisecond
	maxBackoff        = 5 * time.Second // re-registration well within 30 s of a master restart
	snapshotTimeout   = 3 * time.Second
	closeReasonShutdn = 5 // reasonShutdown
)

// Subagent connects to the AgentX master socket of snmpd and serves VRX-MIB until its context ends,
// reconnecting (and re-registering) whenever the master goes away.
type Subagent struct {
	// Socket is the master's unix socket (snmpd.Paths.AgentXSocket).
	Socket string
	Source Source
	Log    *slog.Logger
	// Descr is the Open description (default "vrx-agent VRX-MIB").
	Descr string

	mu         sync.Mutex
	registered bool
	sessions   uint64 // successful registrations (re-registrations included)
	lastErr    string
	since      time.Time
}

// Status is the subagent's state for /state/snmp.
type Status struct {
	Registered    bool      `json:"registered"`
	Registrations uint64    `json:"registrations"`
	Since         time.Time `json:"since,omitzero"`
	LastError     string    `json:"lastError,omitempty"`
}

// Status returns the current state.
func (s *Subagent) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{Registered: s.registered, Registrations: s.sessions, Since: s.since, LastError: s.lastErr}
}

func (s *Subagent) log() *slog.Logger {
	if s.Log == nil {
		return slog.Default()
	}
	return s.Log
}

// Run serves until ctx ends.
func (s *Subagent) Run(ctx context.Context) {
	backoff := minBackoff
	for ctx.Err() == nil {
		start := time.Now()
		err := s.session(ctx)
		s.mu.Lock()
		s.registered = false
		if err != nil && ctx.Err() == nil {
			s.lastErr = err.Error()
		}
		s.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		if time.Since(start) > maxBackoff {
			backoff = minBackoff
		}
		s.log().Debug("AgentX session ended; reconnecting", "socket", s.Socket, "err", err, "in", backoff)
		t := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

// conn is one AgentX session.
type conn struct {
	c       net.Conn
	wmu     sync.Mutex
	session uint32
	packet  uint32
}

func (c *conn) send(typ, flags uint8, txn, packet uint32, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if err := c.c.SetWriteDeadline(time.Now().Add(responseTimeout)); err != nil {
		return err
	}
	_, err := c.c.Write(pdu(typ, flags, c.session, txn, packet, payload))
	return err
}

// request sends an administrative PDU and waits for its Response (before the serve loop runs).
func (c *conn) request(typ uint8, payload []byte) (header, error) {
	c.packet++
	id := c.packet
	if err := c.send(typ, 0, 0, id, payload); err != nil {
		return header{}, err
	}
	if err := c.c.SetReadDeadline(time.Now().Add(responseTimeout)); err != nil {
		return header{}, err
	}
	for {
		h, p, err := readPDU(c.c)
		if err != nil {
			return h, err
		}
		if h.Type != pduResponse || h.PacketID != id {
			continue
		}
		d := decoder{b: p, o: h.order()}
		d.u32() // sysUpTime
		e := d.u16()
		if d.err != nil {
			return h, d.err
		}
		if e != errNone {
			return h, fmt.Errorf("snmpagent: master answered %s with error %d", pduName(typ), e)
		}
		return h, nil
	}
}

func pduName(t uint8) string {
	switch t {
	case pduOpen:
		return "Open"
	case pduRegister:
		return "Register"
	}
	return fmt.Sprintf("PDU %d", t)
}

func (s *Subagent) session(ctx context.Context) error {
	var d net.Dialer
	nc, err := d.DialContext(ctx, "unix", s.Socket)
	if err != nil {
		return err
	}
	c := &conn{c: nc}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			// Best effort Close (reasonShutdown), then drop the socket.
			var e encoder
			e.u8(closeReasonShutdn)
			e.u8(0)
			e.u16(0)
			_ = c.send(pduClose, 0, 0, 0, e.b)
			_ = nc.Close()
		case <-done:
			_ = nc.Close()
		}
	}()

	var e encoder
	e.u8(openTimeoutSec)
	e.u8(0)
	e.u16(0)
	e.oid(VRXMIBOID, false)
	e.octets([]byte(cmpOr(s.Descr, "vrx-agent VRX-MIB")))
	h, err := c.request(pduOpen, e.b)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	c.session = h.SessionID

	e = encoder{}
	e.u8(0) // timeout: session default
	e.u8(DefaultPriority)
	e.u8(0) // range_subid
	e.u8(0)
	e.oid(VRXMIBOID, false)
	if _, err := c.request(pduRegister, e.b); err != nil {
		return fmt.Errorf("register %s: %w", VRXMIBOID, err)
	}
	s.mu.Lock()
	s.registered, s.sessions, s.since, s.lastErr = true, s.sessions+1, time.Now(), ""
	n := s.sessions
	s.mu.Unlock()
	s.log().Info("VRX-MIB registered with the AgentX master", "socket", s.Socket, "subtree", VRXMIBOID.String(), "session", c.session, "registrations", n)

	go func() { // keepalive: a dead master shows up as a write error / EOF
		t := time.NewTicker(pingInterval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				c.wmu.Lock()
				c.packet++
				id := c.packet
				c.wmu.Unlock()
				if err := c.send(pduPing, 0, 0, id, nil); err != nil {
					_ = nc.Close()
					return
				}
			}
		}
	}()
	return s.serve(ctx, c)
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// serve answers the master's requests until the connection ends.
func (s *Subagent) serve(ctx context.Context, c *conn) error {
	for {
		if err := c.c.SetReadDeadline(time.Time{}); err != nil {
			return err
		}
		h, p, err := readPDU(c.c)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		switch h.Type {
		case pduGet, pduGetNext, pduGetBulk:
			resp := s.answer(ctx, h, p)
			if err := c.send(pduResponse, 0, h.TransactionID, h.PacketID, resp); err != nil {
				return err
			}
		case pduTestSet:
			if err := c.send(pduResponse, 0, h.TransactionID, h.PacketID, response(errNotWritable, 1, nil)); err != nil {
				return err
			}
		case pduCommitSet, pduUndoSet, pduCleanupSet:
			if err := c.send(pduResponse, 0, h.TransactionID, h.PacketID, response(errNone, 0, nil)); err != nil {
				return err
			}
		case pduClose:
			return errors.New("snmpagent: master closed the session")
		case pduResponse:
			// answer to our Ping
		default:
			if err := c.send(pduResponse, 0, h.TransactionID, h.PacketID, response(errProcessingErr, 0, nil)); err != nil {
				return err
			}
		}
	}
}

func response(errStatus, index uint16, vbs []VarBind) []byte {
	var e encoder
	e.u32(0) // sysUpTime: unused by a master
	e.u16(errStatus)
	e.u16(index)
	for _, vb := range vbs {
		e.varbind(vb)
	}
	return e.b
}

// answer builds the Response payload of a Get / GetNext / GetBulk.
func (s *Subagent) answer(ctx context.Context, h header, p []byte) []byte {
	if h.Flags&flagNonDefaultContext != 0 {
		return response(errUnsupportedCtx, 0, nil)
	}
	d := decoder{b: p, o: h.order()}
	var nonRep, maxRep int
	if h.Type == pduGetBulk {
		nonRep, maxRep = int(d.u16()), int(d.u16())
	}
	ranges := d.ranges()
	if d.err != nil {
		return response(errParseFailed, 0, nil)
	}
	sctx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	defer cancel()
	snap, err := s.Source.Snapshot(sctx)
	if err != nil {
		s.log().Warn("VRX-MIB snapshot", "err", err)
		return response(errGenErr, 1, nil)
	}
	v := buildView(snap)
	var out []VarBind
	switch h.Type {
	case pduGet:
		for _, r := range ranges {
			out = append(out, v.get(r.Start))
		}
	case pduGetNext:
		for _, r := range ranges {
			out = append(out, v.next(r))
		}
	case pduGetBulk:
		nonRep = min(nonRep, len(ranges))
		for _, r := range ranges[:nonRep] {
			out = append(out, v.next(r))
		}
		rep := ranges[nonRep:]
		cur := append([]searchRange(nil), rep...)
		for range min(maxRep, 64) {
			allEnd := true
			for i := range cur {
				vb := v.next(cur[i])
				out = append(out, vb)
				if vb.Value.Type != TypeEndOfMibView {
					allEnd = false
					cur[i] = searchRange{Start: vb.Name, End: cur[i].End}
				}
			}
			if allEnd || len(cur) == 0 {
				break
			}
		}
	}
	return response(errNone, 0, out)
}
