package ipfixsflow

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// msg builds an IPFIX message with the given sets (id, body).
func msg(domain uint32, sets ...[2]any) []byte {
	var body []byte
	for _, s := range sets {
		id, b := s[0].(uint16), s[1].([]byte)
		h := make([]byte, 4)
		binary.BigEndian.PutUint16(h[0:2], id)
		binary.BigEndian.PutUint16(h[2:4], uint16(4+len(b)))
		body = append(append(body, h...), b...)
	}
	m := make([]byte, 16, 16+len(body))
	binary.BigEndian.PutUint16(m[0:2], 10)
	binary.BigEndian.PutUint16(m[2:4], uint16(16+len(body)))
	binary.BigEndian.PutUint32(m[12:16], domain)
	return append(m, body...)
}

func TestParseMessage(t *testing.T) {
	var s Stats
	tmpl := []byte{0x01, 0x00, 0x00, 0x01, 0x00, 0x08, 0x00, 0x04} // template 256: sourceIPv4Address(8), 4 bytes
	if err := ParseMessage(msg(1, [2]any{uint16(2), tmpl}), &s); err != nil {
		t.Fatal(err)
	}
	if err := ParseMessage(msg(1, [2]any{uint16(256), []byte{10, 1, 1, 1, 10, 1, 1, 2}}), &s); err != nil {
		t.Fatal(err)
	}
	if s.Messages != 2 || s.TemplateSets != 1 || s.DataSets != 1 || s.DataRecords[256] != 8 || !s.Domains[1] {
		t.Fatalf("%+v", s)
	}
	for _, bad := range [][]byte{
		{0, 9, 0, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, // NetFlow v9
		msg(1)[:10],
		func() []byte { m := msg(1, [2]any{uint16(2), tmpl}); m[18] = 0xff; return m }(), // set length out of bounds
		msg(1, [2]any{uint16(5), []byte{}}),                                              // reserved set id
	} {
		if err := ParseMessage(bad, &s); err == nil {
			t.Errorf("accepted % x", bad)
		}
	}
}

func TestCollectorUDP(t *testing.T) {
	c, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	conn, err := net.DialUDP("udp", nil, c.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	for _, m := range [][]byte{msg(7, [2]any{uint16(2), []byte{1, 0, 0, 0}}), msg(7, [2]any{uint16(300), []byte{1, 2, 3, 4}}), {1, 2, 3}} {
		if _, err := conn.Write(m); err != nil {
			t.Fatal(err)
		}
	}
	s, ok := c.WaitFor(5*time.Second, func(s Stats) bool { return s.Messages == 2 && s.Malformed == 1 })
	if !ok || s.TemplateSets != 1 || s.DataSets != 1 || !s.Domains[7] {
		t.Fatalf("%+v", s)
	}
}
