package objects

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
)

// dnsResponder is an in-process DNS server on 127.0.0.1:0 (ephemeral port) for one test zone:
// never port 53, never the host's resolver configuration. It answers A/AAAA queries from records
// (NOERROR with an empty answer = NODATA for a known name without that family, NXDOMAIN for an
// unknown name) and counts queries per name.
type dnsResponder struct {
	t    *testing.T
	conn *net.UDPConn
	addr string

	mu      sync.Mutex
	records map[string][]netip.Addr // lower-case name without the trailing dot
	queries map[string]int
	done    chan struct{}
}

func startDNS(t *testing.T) *dnsResponder {
	t.Helper()
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	d := &dnsResponder{t: t, conn: c, addr: c.LocalAddr().String(), records: map[string][]netip.Addr{}, queries: map[string]int{}, done: make(chan struct{})}
	go d.serve()
	t.Cleanup(d.stop)
	return d
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

func (d *dnsResponder) del(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.records, strings.ToLower(name))
}

func (d *dnsResponder) count(name string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.queries[strings.ToLower(name)]
}

// stop closes the socket: queries then fail at once (ICMP port unreachable → connection refused).
func (d *dnsResponder) stop() {
	select {
	case <-d.done:
		return
	default:
	}
	_ = d.conn.Close()
	<-d.done
}

func (d *dnsResponder) serve() {
	defer close(d.done)
	buf := make([]byte, 1500)
	for {
		n, from, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if resp := d.answer(buf[:n]); resp != nil {
			_, _ = d.conn.WriteToUDP(resp, from)
		}
	}
}

// answer builds the response to one query (RFC 1035 §4.1; the question is echoed, answers point
// to it with a compression pointer).
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
	d.queries[name]++
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
	out.Write(q[0:2])       // id
	flags := uint16(0x8180) // QR, RD, RA, NOERROR
	if !known {
		flags |= 3 // NXDOMAIN
	}
	_ = binary.Write(&out, binary.BigEndian, flags)
	_ = binary.Write(&out, binary.BigEndian, [4]uint16{1, uint16(len(answers)), 0, 0}) //nolint:gosec // a handful of test records
	out.Write(question)
	for _, a := range answers {
		out.Write(a)
	}
	return out.Bytes()
}

// rr is one answer record for the question's name (pointer 0xc00c), class IN, TTL 60.
func rr(typ uint16, data []byte) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, [3]uint16{0xc00c, typ, 1})
	_ = binary.Write(&b, binary.BigEndian, uint32(60))
	_ = binary.Write(&b, binary.BigEndian, uint16(len(data))) //nolint:gosec // 4 or 16 bytes
	b.Write(data)
	return b.Bytes()
}
