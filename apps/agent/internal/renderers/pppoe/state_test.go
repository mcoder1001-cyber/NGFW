package pppoe

import (
	"os"
	"path/filepath"
	"testing"
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
