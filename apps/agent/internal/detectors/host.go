// Package detectors extracts source addresses from trusted host records.
package detectors

import (
	"encoding/json"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Observation is a validated network source and enabled detector kind.
type Observation struct{ Source, Kind string }

// Journal holds kernel-attested journal provenance and the log message.
type Journal struct {
	Cursor    string `json:"__CURSOR"`
	Message   string `json:"MESSAGE"`
	Comm      string `json:"_COMM"`
	UID       string `json:"_UID"`
	Transport string `json:"_TRANSPORT"`
	Timestamp string `json:"__REALTIME_TIMESTAMP"`
}
type peer struct {
	addr string
	at   time.Time
}
type scan struct {
	ports map[uint16]time.Time
	at    time.Time
}

// Host maintains bounded cursor, IKE context and distinct-port windows.
type Host struct {
	peers       map[string]peer
	scans       map[string]*scan
	cursors     map[string]bool
	cursorOrder []string
}

var sshFailure = regexp.MustCompile(`^Failed (?:password|publickey) for (?:invalid user )?.+ from (\S+) port [0-9]+ ssh[12](?: \[preauth\])?$`)
var ikeContext = regexp.MustCompile(`<([^<>]+\|[0-9]+)>`)
var ikePacket = regexp.MustCompile(`received packet: from ([^\s\[\]]+)\[[0-9]+\]`)
var scanSource = regexp.MustCompile(`(?:^| )SRC=([^ ]+)`)
var scanPort = regexp.MustCompile(`(?:^| )DPT=([0-9]+)(?: |$)`)

// NewHost creates an empty host detector with no historical observations.
func NewHost() *Host {
	return &Host{peers: map[string]peer{}, scans: map[string]*scan{}, cursors: map[string]bool{}}
}
func source(raw string) string {
	a, err := netip.ParseAddr(raw)
	if err != nil || a.Zone() != "" || a.IsUnspecified() || a.IsMulticast() || a.IsLoopback() {
		return ""
	}
	return a.Unmap().String()
}

// Observe deduplicates overlapping journal polls and emits one distinct-port
// observation per source within the configured window. IKE auth identities are
// never source addresses: only received-packet addresses correlated by SA ID.
func (h *Host) Observe(line []byte, now time.Time, window time.Duration, threshold, sourceLimit int) *Observation {
	var j Journal
	if json.Unmarshal(line, &j) != nil || j.Cursor == "" {
		return nil
	}
	if h.cursors[j.Cursor] {
		return nil
	}
	h.cursors[j.Cursor] = true
	h.cursorOrder = append(h.cursorOrder, j.Cursor)
	if len(h.cursorOrder) > 20000 {
		delete(h.cursors, h.cursorOrder[0])
		h.cursorOrder = h.cursorOrder[1:]
	}
	us, err := strconv.ParseInt(j.Timestamp, 10, 64)
	if err != nil {
		return nil
	}
	at := time.UnixMicro(us)
	if at.After(now.Add(time.Second)) || now.Sub(at) > 10*time.Second {
		return nil
	}
	if j.UID == "0" && (j.Comm == "sshd" || j.Comm == "sshd-session") {
		m := sshFailure.FindStringSubmatch(j.Message)
		if len(m) > 1 {
			if s := source(m[1]); s != "" {
				return &Observation{s, "ssh"}
			}
		}
		return nil
	}
	if j.UID == "0" && (j.Comm == "charon" || j.Comm == "charon-systemd") {
		ctx := ikeContext.FindStringSubmatch(j.Message)
		if len(ctx) < 2 {
			return nil
		}
		key := ctx[1]
		if packet := ikePacket.FindStringSubmatch(j.Message); len(packet) > 1 {
			if a := source(packet[1]); a != "" {
				for k, p := range h.peers {
					if now.Sub(p.at) > time.Minute {
						delete(h.peers, k)
					}
				}
				if len(h.peers) < 1024 || h.peers[key].addr != "" {
					h.peers[key] = peer{a, now}
				}
			}
			return nil
		}
		if strings.Contains(j.Message, "peer authentication failed") || strings.Contains(j.Message, "EAP authentication failed") || strings.Contains(j.Message, "verification of AUTH payload failed") {
			p := h.peers[key]
			delete(h.peers, key)
			if p.addr != "" && now.Sub(p.at) <= time.Minute {
				return &Observation{p.addr, "vpnAuth"}
			}
		}
		return nil
	}
	if j.Transport != "kernel" || !strings.Contains(j.Message, "ngfw:scan ") || threshold < 1 || window <= 0 {
		return nil
	}
	sm, pm := scanSource.FindStringSubmatch(j.Message), scanPort.FindStringSubmatch(j.Message)
	if len(sm) < 2 || len(pm) < 2 {
		return nil
	}
	a := source(sm[1])
	if a == "" {
		return nil
	}
	pn, err := strconv.ParseUint(pm[1], 10, 16)
	if err != nil || pn == 0 {
		return nil
	}
	port := uint16(pn)
	if sourceLimit < 1 || sourceLimit > 10000 {
		sourceLimit = 10000
	}
	for k, s := range h.scans {
		if now.Sub(s.at) > window {
			delete(h.scans, k)
		}
	}
	s := h.scans[a]
	if s == nil {
		if len(h.scans) >= sourceLimit {
			return nil
		}
		s = &scan{ports: map[uint16]time.Time{}}
		h.scans[a] = s
	}
	s.at = now
	for p, t := range s.ports {
		if now.Sub(t) >= window {
			delete(s.ports, p)
		}
	}
	if _, seen := s.ports[port]; seen {
		return nil
	}
	if len(s.ports) >= threshold {
		s.ports = map[uint16]time.Time{}
	}
	s.ports[port] = now
	return &Observation{a, "portScan"}
}
