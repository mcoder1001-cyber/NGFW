package snmpagent

import (
	"context"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeMaster is a minimal AgentX master: it accepts sessions, answers Open/Register, and lets the test
// send Get/GetNext/GetBulk PDUs (in little-endian byte order, to exercise the decoder's flag handling).
type fakeMaster struct {
	t        *testing.T
	ln       net.Listener
	sessions chan net.Conn
}

func newFakeMaster(t *testing.T) *fakeMaster {
	t.Helper()
	ln, err := net.Listen("unix", filepath.Join(t.TempDir(), "agentx.sock"))
	if err != nil {
		t.Fatal(err)
	}
	m := &fakeMaster{t: t, ln: ln, sessions: make(chan net.Conn, 4)}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			m.handshake(c)
		}
	}()
	return m
}

func respond(c net.Conn, h header, session uint32) {
	var e encoder
	e.u32(0)
	e.u16(0)
	e.u16(0)
	_, _ = c.Write(pdu(pduResponse, 0, session, h.TransactionID, h.PacketID, e.b))
}

func (m *fakeMaster) handshake(c net.Conn) {
	h, p, err := readPDU(c)
	if err != nil || h.Type != pduOpen {
		m.t.Errorf("want Open, got %d %v", h.Type, err)
		return
	}
	d := decoder{b: p, o: h.order()}
	d.u32()
	if id, _ := d.oid(); id.Compare(VRXMIBOID) != 0 {
		m.t.Errorf("Open id %s", id)
	}
	respond(c, h, 42)
	h, p, err = readPDU(c)
	if err != nil || h.Type != pduRegister || h.SessionID != 42 {
		m.t.Errorf("want Register on session 42, got %d/%d %v", h.Type, h.SessionID, err)
		return
	}
	d = decoder{b: p, o: h.order()}
	d.u32()
	if sub, _ := d.oid(); sub.Compare(VRXMIBOID) != 0 {
		m.t.Errorf("Register subtree %s", sub)
	}
	respond(c, h, 42)
	m.sessions <- c
}

// leOID encodes an OID little-endian (the master chose not to set NETWORK_BYTE_ORDER).
func leOID(b []byte, o OID, include bool) []byte {
	inc := byte(0)
	if include {
		inc = 1
	}
	b = append(b, byte(len(o)), 0, inc, 0)
	for _, s := range o {
		b = binary.LittleEndian.AppendUint32(b, s)
	}
	return b
}

func leHeader(typ uint8, packet uint32, payload []byte) []byte {
	b := []byte{1, typ, 0, 0}
	b = binary.LittleEndian.AppendUint32(b, 42)
	b = binary.LittleEndian.AppendUint32(b, 7)
	b = binary.LittleEndian.AppendUint32(b, packet)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(payload)))
	return append(b, payload...)
}

func ask(t *testing.T, c net.Conn, typ uint8, packet uint32, payload []byte) []VarBind {
	t.Helper()
	if _, err := c.Write(leHeader(typ, packet, payload)); err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	h, p, err := readPDU(c)
	if err != nil {
		t.Fatal(err)
	}
	if h.Type != pduResponse || h.PacketID != packet || h.TransactionID != 7 {
		t.Fatalf("response header %+v", h)
	}
	d := decoder{b: p, o: h.order()}
	d.u32()
	if e := d.u16(); e != errNone {
		t.Fatalf("response error %d", e)
	}
	d.u16()
	var out []VarBind
	for d.err == nil && len(d.b) > 0 {
		out = append(out, d.varbind())
	}
	if d.err != nil {
		t.Fatal(d.err)
	}
	return out
}

var testSnap = Snapshot{
	Agent: AgentInfo{Version: "0.1.0", VppConnected: true, Revision: "txn-17", Commits: 3},
	Interfaces: []Interface{
		{SwIfIndex: 1, Name: "loop0", AdminUp: true, OperUp: true, InOctets: 1 << 40, OutOctets: 20, InPkts: 3, OutPkts: 4, InErrors: 5, OutErrors: 6},
		{SwIfIndex: 0, Name: "local0"},
	},
}

func startSubagent(t *testing.T, m *fakeMaster) (*Subagent, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := &Subagent{Socket: m.ln.Addr().String(), Source: SourceFunc(func(context.Context) (Snapshot, error) { return testSnap, nil })}
	go s.Run(ctx)
	t.Cleanup(cancel)
	return s, cancel
}

func waitSession(t *testing.T, m *fakeMaster, within time.Duration) net.Conn {
	t.Helper()
	select {
	case c := <-m.sessions:
		return c
	case <-time.After(within):
		t.Fatalf("no AgentX registration within %v", within)
	}
	return nil
}

// TestSubagentWalk: Open + Register, then a full GetNext walk of VRX-MIB, a Get and a GetBulk.
func TestSubagentWalk(t *testing.T) {
	m := newFakeMaster(t)
	s, _ := startSubagent(t, m)
	c := waitSession(t, m, 5*time.Second)
	waitStatus(t, s, 1)

	var walked []VarBind
	cur := VRXMIBOID
	for i := uint32(1); i < 100; i++ {
		vbs := ask(t, c, pduGetNext, i, leOID(leOID(nil, cur, false), nil, false))
		if len(vbs) != 1 {
			t.Fatalf("GetNext returned %d varbinds", len(vbs))
		}
		if vbs[0].Value.Type == TypeEndOfMibView {
			break
		}
		walked = append(walked, vbs[0])
		cur = vbs[0].Name
	}
	if want := 5 + 10*2; len(walked) != want {
		t.Fatalf("walk returned %d varbinds, want %d", len(walked), want)
	}
	for i, vb := range walked {
		t.Logf("%s = type %d int %d u64 %d str %q", vb.Name, vb.Value.Type, vb.Value.Int, vb.Value.U64, vb.Value.Str)
		if i > 0 && walked[i-1].Name.Compare(vb.Name) >= 0 {
			t.Fatalf("walk not strictly increasing at %s", vb.Name)
		}
	}
	// vrxIfName.2 (sw_if_index 1) and vrxIfInOctets.2 (a Counter64 above 2^32)
	name := MustOID(VRXMIBOID.String() + ".2.1.2.2")
	oct := MustOID(VRXMIBOID.String() + ".2.1.5.2")
	vbs := ask(t, c, pduGet, 200, leOID(leOID(leOID(leOID(nil, name, false), nil, false), oct, false), nil, false))
	if string(vbs[0].Value.Str) != "loop0" || vbs[1].Value.Type != TypeCounter64 || vbs[1].Value.U64 != 1<<40 {
		t.Fatalf("Get: %+v", vbs)
	}
	missing := ask(t, c, pduGet, 201, leOID(leOID(nil, MustOID(VRXMIBOID.String()+".2.1.2.99"), false), nil, false))
	if missing[0].Value.Type != TypeNoSuchInstance {
		t.Fatalf("Get of a missing row: type %d", missing[0].Value.Type)
	}
	// GetBulk: 0 non-repeaters, 3 repetitions from vrxIfName → names then the admin column.
	bulk := binary.LittleEndian.AppendUint16(nil, 0)
	bulk = binary.LittleEndian.AppendUint16(bulk, 3)
	bulk = leOID(leOID(bulk, MustOID(VRXMIBOID.String()+".2.1.2"), false), nil, false)
	vbs = ask(t, c, pduGetBulk, 202, bulk)
	if len(vbs) != 3 || string(vbs[0].Value.Str) != "local0" || string(vbs[1].Value.Str) != "loop0" || vbs[2].Value.Int != statusDown {
		t.Fatalf("GetBulk: %+v", vbs)
	}
	// SET is refused (read-only MIB).
	if _, err := c.Write(leHeader(pduTestSet, 203, nil)); err != nil {
		t.Fatal(err)
	}
	h, p, err := readPDU(c)
	if err != nil {
		t.Fatal(err)
	}
	d := decoder{b: p, o: h.order()}
	d.u32()
	if e := d.u16(); e != errNotWritable {
		t.Fatalf("TestSet answered %d, want notWritable", e)
	}
}

// TestSubagentReregisters: the master drops the session (snmpd restart) → the subagent re-opens and
// re-registers well within 30 s.
func TestSubagentReregisters(t *testing.T) {
	m := newFakeMaster(t)
	s, _ := startSubagent(t, m)
	c := waitSession(t, m, 5*time.Second)
	start := time.Now()
	_ = c.Close()
	c2 := waitSession(t, m, 30*time.Second)
	defer c2.Close()
	took := time.Since(start)
	waitStatus(t, s, 2)
	t.Logf("re-registered after master restart in %v", took)
}

// TestOIDEncodingRoundTrip covers the prefix compression of RFC 2741 §5.1.
func TestOIDEncodingRoundTrip(t *testing.T) {
	for _, s := range []string{".1.3.6.1.4.1.8072.9999.9999.7853.2.1.5.2", ".1.3.6.1.2.1.1.5.0", ".1.2", ".1.3.6.1.300.1"} {
		var e encoder
		e.oid(MustOID(s), true)
		d := decoder{b: e.b, o: binary.BigEndian}
		o, inc := d.oid()
		if o.String() != s || !inc || d.err != nil {
			t.Fatalf("%s → %s (%v, %v)", s, o, inc, d.err)
		}
	}
}

func waitStatus(t *testing.T, s *Subagent, registrations uint64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := s.Status()
		if st.Registered && st.Registrations == registrations {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("status %+v, want registered #%d", st, registrations)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestProductSourceRevision: the running revision and commit counter come from agent-state.json;
// without a stats segment the table is empty, not an error.
func TestProductSourceRevision(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "agent-state.json")
	write := func(s string) {
		if err := os.WriteFile(f, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"last_txn_id":"t1","history":[{},{}]}`)
	p := &ProductSource{StatsSocket: filepath.Join(dir, "none.sock"), StateFile: f, Version: "v", Connected: func() bool { return false }}
	s, err := p.Snapshot(context.Background())
	if err != nil || s.Agent.Revision != "t1" || s.Agent.Commits != 2 || len(s.Interfaces) != 0 {
		t.Fatalf("%+v %v", s, err)
	}
	write(`{"last_txn_id":"t2","history":[{},{},{}]}`)
	s, _ = p.Snapshot(context.Background())
	if s.Agent.Revision != "t2" || s.Agent.Commits != 3 {
		t.Fatalf("%+v", s.Agent)
	}
}
