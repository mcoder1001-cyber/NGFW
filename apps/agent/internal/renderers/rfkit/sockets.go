package rfkit

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// UDPListeners returns the unconnected UDP sockets process pid holds, as "udp:<ip>:<port>" /
// "udp6:[<ip>]:<port>" (snmpd's transport spelling), sorted. It is the proof of what a daemon
// actually listens on — a reload may re-read a file without reopening sockets (RF-4 review H1).
// Sockets bound to a port in the kernel's ephemeral range (outgoing client sockets, e.g. trap
// sessions) are left out. Reads /proc/<pid>/fd (root) and the process's own net namespace view
// /proc/<pid>/net/udp{,6}.
func UDPListeners(pid int) ([]string, error) {
	root := procRoot()
	fdDir := filepath.Join(root, strconv.Itoa(pid), "fd")
	ents, err := os.ReadDir(fdDir)
	if err != nil {
		return nil, fmt.Errorf("rfkit: sockets of pid %d: %w", pid, err)
	}
	inodes := map[string]bool{}
	for _, e := range ents {
		l, err := os.Readlink(filepath.Join(fdDir, e.Name()))
		if err == nil && strings.HasPrefix(l, "socket:[") {
			inodes[strings.TrimSuffix(strings.TrimPrefix(l, "socket:["), "]")] = true
		}
	}
	lo, hi := ephemeralRange(root)
	var out []string
	for _, fam := range []struct{ file, scheme string }{{"udp", "udp"}, {"udp6", "udp6"}} {
		b, err := ReadFileLimit(filepath.Join(root, strconv.Itoa(pid), "net", fam.file), 4<<20)
		if err != nil {
			continue // no IPv6 in this namespace
		}
		sc := bufio.NewScanner(bytes.NewReader(b))
		sc.Buffer(make([]byte, 64<<10), 64<<10)
		first := true
		for sc.Scan() {
			if first {
				first = false
				continue
			}
			f := strings.Fields(sc.Text())
			// sl local_address rem_address st tx:rx tr:tm retrnsmt uid timeout inode
			if len(f) < 10 || !inodes[f[9]] {
				continue
			}
			rem := f[2]
			if strings.Trim(strings.Split(rem, ":")[0], "0") != "" {
				continue // connected (client) socket
			}
			ap, err := parseProcAddr(f[1])
			if err != nil {
				continue
			}
			if p := int(ap.Port()); p >= lo && p <= hi {
				continue
			}
			if fam.scheme == "udp" {
				out = append(out, fmt.Sprintf("udp:%s:%d", ap.Addr(), ap.Port()))
			} else {
				out = append(out, fmt.Sprintf("udp6:[%s]:%d", ap.Addr(), ap.Port()))
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

func ephemeralRange(root string) (int, int) {
	b, err := ReadFileLimit(filepath.Join(root, "sys/net/ipv4/ip_local_port_range"), 64)
	if err == nil {
		f := strings.Fields(string(b))
		if len(f) == 2 {
			lo, e1 := strconv.Atoi(f[0])
			hi, e2 := strconv.Atoi(f[1])
			if e1 == nil && e2 == nil {
				return lo, hi
			}
		}
	}
	return 32768, 60999
}

// parseProcAddr decodes "0100007F:0EFD" (IPv4) or the 32-hex-digit IPv6 form of /proc/net/udp*:
// the address is stored as host-order (little-endian) 32-bit words.
func parseProcAddr(s string) (netip.AddrPort, error) {
	h, p, ok := strings.Cut(s, ":")
	if !ok {
		return netip.AddrPort{}, fmt.Errorf("bad address %q", s)
	}
	port, err := strconv.ParseUint(p, 16, 16)
	if err != nil {
		return netip.AddrPort{}, err
	}
	raw, err := hex.DecodeString(h)
	if err != nil || (len(raw) != 4 && len(raw) != 16) {
		return netip.AddrPort{}, fmt.Errorf("bad address %q", s)
	}
	for i := 0; i+4 <= len(raw); i += 4 {
		raw[i], raw[i+1], raw[i+2], raw[i+3] = raw[i+3], raw[i+2], raw[i+1], raw[i]
	}
	a, _ := netip.AddrFromSlice(raw)
	return netip.AddrPortFrom(a, uint16(port)), nil
}
