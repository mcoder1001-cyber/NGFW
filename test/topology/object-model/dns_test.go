package objectmodel

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
)

// dnsResponder is the test's own DNS server for the test-only zone <prefix>.test on 127.0.0.1:<ephemeral> — never
// port 53, never the host's Unbound or systemd-resolved configuration. It answers A/AAAA from records (NXDOMAIN for
// unknown names, NOERROR/no answer for a family a name lacks), TTL 60, and counts queries. stop() closes the socket
// (the agent's queries then fail at once); startAgain() listens on the same address again.
type dnsResponder struct {
	addr string

	mu      sync.Mutex
	conn    *net.UDPConn
	done    chan struct{}
	records map[string][]netip.Addr
	queries int
}

func startDNS(t *testing.T) *dnsResponder {
	t.Helper()
	d := &dnsResponder{records: map[string][]netip.Addr{}}
	if err := d.listen("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.stop)
	return d
}

func (d *dnsResponder) listen(addr string) error {
	ua, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	c, err := net.ListenUDP("udp", ua)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.conn, d.done, d.addr = c, make(chan struct{}), c.LocalAddr().String()
	done := d.done
	d.mu.Unlock()
	go d.serve(c, done)
	return nil
}

func (d *dnsResponder) startAgain(t *testing.T) {
	t.Helper()
	if err := d.listen(d.addr); err != nil {
		t.Fatalf("DNS responder on %s again: %v", d.addr, err)
	}
}

func (d *dnsResponder) stop() {
	d.mu.Lock()
	c, done := d.conn, d.done
	d.conn = nil
	d.mu.Unlock()
	if c != nil {
		_ = c.Close()
		<-done
	}
}

func (d *dnsResponder) set(name string, addrs ...string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var as []netip.Addr
	for _, a := range addrs {
		as = append(as, netip.MustParseAddr(a))
	}
	d.records[strings.ToLower(name)] = as
}

func (d *dnsResponder) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.queries
}

func (d *dnsResponder) serve(c *net.UDPConn, done chan struct{}) {
	defer close(done)
	buf := make([]byte, 1500)
	for {
		n, from, err := c.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if resp := d.answer(buf[:n]); resp != nil {
			_, _ = c.WriteToUDP(resp, from)
		}
	}
}

// answer builds the response to one query (RFC 1035 §4.1: the question is echoed, answers point to it).
func (d *dnsResponder) answer(q []byte) []byte {
	if len(q) < 12 || binary.BigEndian.Uint16(q[4:6]) != 1 {
		return nil
	}
	off := 12
	var labels []string
	for {
		if off >= len(q) {
			return nil
		}
		l := int(q[off])
		off++
		if l == 0 {
			break
		}
		if l > 63 || off+l > len(q) {
			return nil
		}
		labels = append(labels, string(q[off:off+l]))
		off += l
	}
	if off+4 > len(q) {
		return nil
	}
	qtype := binary.BigEndian.Uint16(q[off : off+2])
	question := q[12 : off+4]
	name := strings.ToLower(strings.Join(labels, "."))
	d.mu.Lock()
	d.queries++
	addrs, known := d.records[name]
	d.mu.Unlock()
	var answers [][]byte
	for _, a := range addrs {
		switch {
		case qtype == 1 && a.Is4():
			b := a.As4()
			answers = append(answers, rr(1, b[:]))
		case qtype == 28 && a.Is6():
			b := a.As16()
			answers = append(answers, rr(28, b[:]))
		}
	}
	var out bytes.Buffer
	out.Write(q[0:2])
	flags := uint16(0x8180) // QR, RD, RA, NOERROR
	if !known {
		flags |= 3 // NXDOMAIN
	}
	_ = binary.Write(&out, binary.BigEndian, flags)
	_ = binary.Write(&out, binary.BigEndian, [4]uint16{1, uint16(len(answers)), 0, 0}) //nolint:gosec // a few records
	out.Write(question)
	for _, a := range answers {
		out.Write(a)
	}
	return out.Bytes()
}

func rr(typ uint16, data []byte) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, [3]uint16{0xc00c, typ, 1})
	_ = binary.Write(&b, binary.BigEndian, uint32(60))
	_ = binary.Write(&b, binary.BigEndian, uint16(len(data))) //nolint:gosec // 4 or 16 bytes
	b.Write(data)
	return b.Bytes()
}
