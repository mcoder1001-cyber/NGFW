package pppoe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeState(t *testing.T, r *Renderer, hostIf, body string) {
	t.Helper()
	dir := r.paths.StateDir
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, hostIf+".state"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadState(t *testing.T) {
	r := New(WithPaths(PathsUnder(t.TempDir())))

	// missing file → down
	st, err := r.ReadState("wan0", 0, "")
	if err != nil || st.GetPhase() != "down" {
		t.Fatalf("missing: %v %v", st, err)
	}

	// an up session with addresses and DNS
	writeState(t, r, "wan0", "phase=up\nhostif=wan0\nppp_iface=ppp0\nlocal=203.0.113.5\npeer=203.0.113.1\ndns1=203.0.113.53\ndns2=203.0.113.54\nat=2026-09-27T10:00:00Z\n")
	st, err = r.ReadState("wan0", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if st.GetPhase() != "up" || st.GetLocalIpv4() != "203.0.113.5/32" || st.GetPeerIpv4() != "203.0.113.1" {
		t.Fatalf("addresses: %+v", st)
	}
	if len(st.GetDns()) != 2 || st.GetDns()[0] != "203.0.113.53" {
		t.Fatalf("dns: %v", st.GetDns())
	}
	if st.GetSince() == nil {
		t.Fatal("since not set for an up session")
	}

	// down with failures → failed
	writeState(t, r, "wan1", "phase=down\nhostif=wan1\n")
	st, err = r.ReadState("wan1", 3, "auth failed")
	if err != nil {
		t.Fatal(err)
	}
	if st.GetPhase() != "failed" || st.GetFailCount() != 3 || st.GetLastError() != "auth failed" {
		t.Fatalf("failed: %+v", st)
	}
}

func writeState6(t *testing.T, r *Renderer, hostIf, body string) {
	t.Helper()
	if err := os.MkdirAll(r.paths.StateDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.paths.StateDir, hostIf+".state6"), []byte(body), 0o600); err != nil { // #nosec G703 -- Test-only private TempDir renderer and literal fixture interface names; no external path input.
		t.Fatal(err)
	}
}

func TestReadIPv6(t *testing.T) {
	r := New(WithPaths(PathsUnder(t.TempDir())))
	if st, err := r.ReadIPv6("wan0"); err != nil || st.Up {
		t.Fatalf("missing: %+v %v", st, err)
	}
	// Values come from the network: only well-formed, correctly scoped ones survive.
	writeState6(t, r, "wan0", "phase=up\nppp_iface=ppp0\nlllocal=fe80::494e:09cd:477c:b51c\nllremote=fe80::940e:fe14:36be:d2f3\n"+
		"addr=2001:db8:9:0:494e:9cd:477c:b51c/64\naddr=2001:db8:9::100/128\naddr=fe80::1/64\naddr=::ffff:192.0.2.1/128\naddr=junk\n"+
		"addr=2001:db8:9::100/128\ngw=fe80::940e:fe14:36be:d2f3\npd=2001:db8:9100::1/56\nat=2026-10-07T05:32:35Z\n"+pdLeaseFields(time.Now().Add(time.Hour), time.Now().Add(30*time.Minute)))
	st, err := r.ReadIPv6("wan0")
	if err != nil || !st.Up {
		t.Fatal(st, err)
	}
	if len(st.Addrs) != 2 || st.Addrs[0].String() != "2001:db8:9::100/128" || st.Addrs[1].String() != "2001:db8:9:0:494e:9cd:477c:b51c/64" {
		t.Fatalf("addrs %v", st.Addrs)
	}
	if got := st.HostAddrs(); len(got) != 2 || got[1] != "2001:db8:9:0:494e:9cd:477c:b51c/128" {
		t.Fatalf("host addrs %v", got)
	}
	if st.Gateway.String() != "fe80::940e:fe14:36be:d2f3" || st.Delegated.String() != "2001:db8:9100::/56" || st.LinkLocal.String() != "fe80::494e:9cd:477c:b51c" {
		t.Fatalf("gw %v pd %v ll %v", st.Gateway, st.Delegated, st.LinkLocal)
	}
	if st.Summary() != "2001:db8:9::100/128, 2001:db8:9:0:494e:9cd:477c:b51c/64, delegated 2001:db8:9100::/56" {
		t.Fatalf("summary %q", st.Summary())
	}
	writeState6(t, r, "wan0", "phase=up\ngw=203.0.113.1\npd=2001:db8::/8\n")
	if st, _ := r.ReadIPv6("wan0"); st.Gateway.IsValid() || st.Delegated.IsValid() {
		t.Fatalf("IPv4 gateway / implausible prefix accepted: %+v", st)
	}
	// down: nothing negotiated is current
	writeState6(t, r, "wan0", "phase=down\naddr=2001:db8:9::100/128\ngw=fe80::1\n")
	if st, _ := r.ReadIPv6("wan0"); st.Up || len(st.Addrs) != 0 || st.Gateway.IsValid() {
		t.Fatalf("down kept values: %+v", st)
	}
}

func TestReadStateIncludesIPv6(t *testing.T) {
	r := New(WithPaths(PathsUnder(t.TempDir())))
	// dual stack: IPv4 hook up, IPv6 summary filled
	writeState(t, r, "wan0", "phase=up\nlocal=203.0.113.5\npeer=203.0.113.1\n")
	writeState6(t, r, "wan0", "phase=up\naddr=2001:db8:9::5/64\ngw=fe80::1\npd=2001:db8:9100::/56\n"+pdLeaseFields(time.Now().Add(time.Hour), time.Now().Add(30*time.Minute)))
	st, err := r.ReadState("wan0", 0, "")
	if err != nil || st.GetPhase() != "up" || st.GetIpv6() != "2001:db8:9::5/64, delegated 2001:db8:9100::/56" || st.GetLocalIpv4() != "203.0.113.5/32" {
		t.Fatalf("%+v %v", st, err)
	}
	// IPv6-only session (no IPCP): still up, with its own since
	writeState6(t, r, "wan1", "phase=up\naddr=2001:db8:9::6/64\nat=2026-10-07T05:00:00Z\n")
	st, err = r.ReadState("wan1", 2, "x")
	if err != nil || st.GetPhase() != "up" || st.GetIpv6() != "2001:db8:9::6/64" || st.GetSince() == nil || st.GetLocalIpv4() != "" {
		t.Fatalf("v6-only: %+v %v", st, err)
	}
	// IPv6 down, IPv4 down with failures → failed, no IPv6 text
	writeState(t, r, "wan2", "phase=down\n")
	writeState6(t, r, "wan2", "phase=down\naddr=2001:db8:9::7/64\n")
	st, _ = r.ReadState("wan2", 1, "peer did not respond")
	if st.GetPhase() != "failed" || st.GetIpv6() != "" {
		t.Fatalf("down: %+v", st)
	}
}

func pdLeaseFields(valid, preferred time.Time) string {
	return fmt.Sprintf("pd_generation=%s\npd_valid_until=%d\npd_preferred_until=%d\n", strings.Repeat("a", 64), valid.Unix(), preferred.Unix())
}
func TestReadIPv6DelegationExpiry(t *testing.T) {
	r := New(WithPaths(PathsUnder(t.TempDir())))
	for name, metadata := range map[string]string{
		"missing": "", "expired": pdLeaseFields(time.Now().Add(-time.Minute), time.Now().Add(-time.Hour)),
		"reversed":           pdLeaseFields(time.Now().Add(time.Minute), time.Now().Add(time.Hour)),
		"invalid-generation": "pd_generation=evil\npd_valid_until=9999999999\npd_preferred_until=9999999998\n",
	} {
		t.Run(name, func(t *testing.T) {
			writeState6(t, r, "wan0", "phase=up\npd=2001:db8::/56\n"+metadata)
			st, err := r.ReadIPv6("wan0")
			if err != nil || st.Delegated.IsValid() {
				t.Fatalf("stale lease: %+v %v", st, err)
			}
		})
	}
}
