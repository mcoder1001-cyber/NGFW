package nat44ei6466nptv6

// IPv6 traffic of the NAT64 and NPTv6 packet tests (the IPv4 helpers are traffic_test.go, copied from F-nat44-ed-sessions)
// and the IPv6 side of the rig, which tools/lab leaves IPv4-only: addresses inside fd00:<slot hex>::/32 on the
// namespace veths, router advertisements and autoconfiguration off, removed in Cleanup.

import (
	"regexp"
	"strings"
	"testing"
)

// holdServer6 is holdServer on an IPv6 socket.
const holdServer6 = `import socket, sys
s = socket.socket(socket.AF_INET6)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind((sys.argv[1], int(sys.argv[2])))
s.listen(64)
held = []
while True:
    c, _ = s.accept()
    try:
        c.sendall(b"vrx-nat-ok\n")
    except OSError:
        pass
    held.append(c)
`

// connectOnce6 connects over IPv6 from a fixed source, prints the greeting (proof the far end answered) and exits.
const connectOnce6 = `import socket, sys
c = socket.socket(socket.AF_INET6)
c.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
c.bind((sys.argv[1], int(sys.argv[2])))
c.settimeout(5)
c.connect((sys.argv[3], int(sys.argv[4])))
print(c.recv(64).decode().strip())
c.close()
`

// holdClient6 is holdClient over IPv6 (the session stays in the NAT64 table while the test reads it).
const holdClient6 = `import socket, sys, time
c = socket.socket(socket.AF_INET6)
c.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
c.bind((sys.argv[1], int(sys.argv[2])))
c.settimeout(5)
c.connect((sys.argv[3], int(sys.argv[4])))
print(c.recv(64).decode().strip(), flush=True)
time.sleep(float(sys.argv[5]) if len(sys.argv) > 5 else 600)
`

var tcpdumpIP6 = regexp.MustCompile(` IP6 ([0-9a-f:]+)\.(\d+) > ([0-9a-f:]+)\.(\d+):`)

// srcOf6 returns the source address and port of an IPv6 tcpdump line.
func srcOf6(line string) (string, string) {
	m := tcpdumpIP6.FindStringSubmatch(line)
	if m == nil {
		return "", ""
	}
	return m[1], m[2]
}

// v6 is the IPv6 plan of the rig for this slot (all inside fd00:<slot hex>::/32).
type v6 struct {
	lanGW, lanClient   string // NAT64: fd00:N:1::1 (VPP) / ::2 (client)
	nptGW, nptHost     string // NPTv6 internal: fd00:N:10::1 (VPP) / ::2 (host)
	wanGW, wanHost     string // fd00:N:2::1 (VPP) / ::2 (wan host)
	nptInternal        string // fd00:N:10::/48
	nptExternal        string // fd00:N:20::/48
	nat64Prefix        string // fd00:N:64::/96
	nat66Local, nat66X string // NAT66 static mapping (on the slot loopbacks)
}

func newV6(s slot) v6 {
	h := hexSlot(s)
	p := "fd00:" + h + ":"
	return v6{
		lanGW: p + "1::1", lanClient: p + "1::2",
		nptGW: p + "10::1", nptHost: p + "10::2",
		wanGW: p + "2::1", wanHost: p + "2::2",
		nptInternal: p + "10::/48", nptExternal: p + "20::/48",
		nat64Prefix: p + "64::/96",
		nat66Local:  p + "66:1::66", nat66X: p + "66:2::66",
	}
}

func hexSlot(s slot) string {
	const digits = "0123456789abcdef"
	n := s.num
	if n < 16 {
		return string(digits[n])
	}
	return string(digits[n/16]) + string(digits[n%16])
}

// synth is the NAT64 address of an IPv4 host in a /96 prefix ("fd00:4:64::/96", "10.4.2.2" → "fd00:4:64::a04:202").
func synth(prefix96, ip4 string) string {
	p := strings.TrimSuffix(prefix96, "/96")
	o := strings.Split(ip4, ".")
	if len(o) != 4 {
		return ""
	}
	b := make([]int, 4)
	for i, x := range o {
		for _, c := range x {
			b[i] = b[i]*10 + int(c-'0')
		}
	}
	hex := func(v int) string {
		const digits = "0123456789abcdef"
		if v == 0 {
			return "0"
		}
		var s []byte
		for v > 0 {
			s = append([]byte{digits[v%16]}, s...)
			v /= 16
		}
		return string(s)
	}
	return p + hex(b[0]<<8|b[1]) + ":" + hex(b[2]<<8|b[3])
}

// up6 gives the namespace veths their IPv6 addresses and routes (RA and autoconf off, no DAD) and removes them in
// Cleanup. The lan host carries the NAT64 client and the NPTv6 internal host; the wan host routes the NPTv6 external
// prefix back through VPP. The hosts start every exchange (warm-up pings to the gateways), so VPP learns them from their
// multicast neighbour solicitations: VPP's own solicitations come from its link-local address, and an answer to that
// address on a NAT64 inside interface would be taken by nat64-in2out (it translates every unicast packet that is not
// for an interface address).
func (r rig) up6(t *testing.T, a v6) {
	t.Helper()
	for _, x := range []struct{ ns, dev string }{{r.lanNS, r.lanPeer}, {r.wanNS, r.wanPeer}} {
		for _, kv := range []string{"accept_ra=0", "autoconf=0", "accept_dad=0", "disable_ipv6=0"} {
			mustRun(t, "ip", "netns", "exec", x.ns, "sysctl", "-qw", "net.ipv6.conf."+x.dev+"."+kv)
		}
	}
	mustRun(t, "ip", "-n", r.lanNS, "-6", "addr", "add", a.lanClient+"/64", "dev", r.lanPeer, "nodad")
	mustRun(t, "ip", "-n", r.lanNS, "-6", "addr", "add", a.nptHost+"/64", "dev", r.lanPeer, "nodad")
	mustRun(t, "ip", "-n", r.wanNS, "-6", "addr", "add", a.wanHost+"/64", "dev", r.wanPeer, "nodad")
	mustRun(t, "ip", "-n", r.lanNS, "-6", "route", "replace", a.nat64Prefix, "via", a.lanGW, "src", a.lanClient)
	mustRun(t, "ip", "-n", r.lanNS, "-6", "route", "replace", strings.TrimSuffix(a.wanHost, "::2")+"::/64", "via", a.nptGW, "src", a.nptHost)
	mustRun(t, "ip", "-n", r.wanNS, "-6", "route", "replace", a.nptExternal, "via", a.wanGW)
	t.Cleanup(func() {
		for _, c := range [][]string{
			{"-n", r.lanNS, "-6", "route", "del", a.nat64Prefix},
			{"-n", r.lanNS, "-6", "route", "del", strings.TrimSuffix(a.wanHost, "::2") + "::/64"},
			{"-n", r.wanNS, "-6", "route", "del", a.nptExternal},
			{"-n", r.lanNS, "-6", "addr", "del", a.lanClient + "/64", "dev", r.lanPeer},
			{"-n", r.lanNS, "-6", "addr", "del", a.nptHost + "/64", "dev", r.lanPeer},
			{"-n", r.wanNS, "-6", "addr", "del", a.wanHost + "/64", "dev", r.wanPeer},
		} {
			_, _ = run(t, "ip", c...)
		}
		for _, x := range []struct{ ns, dev string }{{r.lanNS, r.lanPeer}, {r.wanNS, r.wanPeer}} {
			_, _ = run(t, "ip", "netns", "exec", x.ns, "sysctl", "-qw", "net.ipv6.conf."+x.dev+".disable_ipv6=1")
		}
		t.Log("rig IPv6 removed (addresses, routes; IPv6 disabled again on the namespace veths)")
	})
}
