// Package ipfixsflow is F-ipfix-sflow's topology evidence: a tiny UDP IPFIX collector (hsflowd, nc and socat
// are not installed on the host; no package installs) and the opt-in host test that points VPP's exporter 0
// at it inside a globals window (D-082).
package ipfixsflow

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// Stats is what the collector saw.
type Stats struct {
	Messages     int            // IPFIX messages (version 10)
	TemplateSets int            // set id 2
	OptionSets   int            // set id 3
	DataSets     int            // set id ≥ 256
	DataRecords  map[uint16]int // bytes of data per template id (records are template-sized)
	Domains      map[uint32]bool
	Malformed    int
}

// ParseMessage decodes one IPFIX message (RFC 7011 §3) into s.
func ParseMessage(b []byte, s *Stats) error {
	if len(b) < 16 {
		return errors.New("short message")
	}
	if v := binary.BigEndian.Uint16(b[0:2]); v != 10 {
		return fmt.Errorf("version %d, want 10 (IPFIX)", v)
	}
	n := int(binary.BigEndian.Uint16(b[2:4]))
	if n != len(b) {
		return fmt.Errorf("length field %d, datagram %d", n, len(b))
	}
	if s.DataRecords == nil {
		s.DataRecords = map[uint16]int{}
	}
	if s.Domains == nil {
		s.Domains = map[uint32]bool{}
	}
	s.Domains[binary.BigEndian.Uint32(b[12:16])] = true
	for off := 16; off < n; {
		if off+4 > n {
			return errors.New("truncated set header")
		}
		id := binary.BigEndian.Uint16(b[off : off+2])
		l := int(binary.BigEndian.Uint16(b[off+2 : off+4]))
		if l < 4 || off+l > n {
			return fmt.Errorf("set %d length %d out of bounds", id, l)
		}
		switch {
		case id == 2:
			s.TemplateSets++
		case id == 3:
			s.OptionSets++
		case id >= 256:
			s.DataSets++
			s.DataRecords[id] += l - 4
		default:
			return fmt.Errorf("reserved set id %d", id)
		}
		off += l
	}
	s.Messages++
	return nil
}

// Collector is a UDP listener that parses every datagram as IPFIX.
type Collector struct {
	conn *net.UDPConn
	mu   sync.Mutex
	st   Stats
	done chan struct{}
}

// Listen starts a collector on addr (e.g. "127.0.0.1:3171").
func Listen(addr string) (*Collector, error) {
	ua, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", ua)
	if err != nil {
		return nil, err
	}
	c := &Collector{conn: conn, done: make(chan struct{})}
	go c.loop()
	return c, nil
}

func (c *Collector) loop() {
	defer close(c.done)
	buf := make([]byte, 65536)
	for {
		n, _, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		c.mu.Lock()
		if ParseMessage(buf[:n], &c.st) != nil {
			c.st.Malformed++
		}
		c.mu.Unlock()
	}
}

// Addr is the bound address.
func (c *Collector) Addr() *net.UDPAddr { return c.conn.LocalAddr().(*net.UDPAddr) }

// Snapshot returns a copy of the statistics.
func (c *Collector) Snapshot() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.st
	s.DataRecords = map[uint16]int{}
	for k, v := range c.st.DataRecords {
		s.DataRecords[k] = v
	}
	s.Domains = map[uint32]bool{}
	for k, v := range c.st.Domains {
		s.Domains[k] = v
	}
	return s
}

// WaitFor polls until ok(Snapshot()) or the timeout.
func (c *Collector) WaitFor(timeout time.Duration, ok func(Stats) bool) (Stats, bool) {
	deadline := time.Now().Add(timeout)
	for {
		s := c.Snapshot()
		if ok(s) || time.Now().After(deadline) {
			return s, ok(s)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Close stops the collector.
func (c *Collector) Close() error {
	err := c.conn.Close()
	<-c.done
	return err
}
