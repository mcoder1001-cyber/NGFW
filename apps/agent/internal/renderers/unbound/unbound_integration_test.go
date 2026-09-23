package unbound

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/vpp/vpptest"
)

// Integration: a real `unbound -d` child with test-scoped paths under
// /run/vrx-test/<prefix>/unbound, listening on 127.0.0.1:3<slot>53 only. Never the system
// unit, never /etc/unbound.

const unboundBin = "/usr/sbin/unbound"

func TestUnboundIntegration(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	prefix, slot := vpptest.Prefix(t), vpptest.Slot(t)
	port := uint32(3000 + slot*100 + 53) //nolint:gosec // slots 1–12
	paths := TestPaths(prefix)
	if err := os.MkdirAll(paths.ConfDir, 0o750); err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile(paths.RootKey)
	if err != nil {
		t.Fatalf("dns-root-data: %v", err)
	}
	if err := os.WriteFile(paths.TrustAnchor, key, 0o644); err != nil { //nolint:gosec // public trust anchor
		t.Fatal(err)
	}
	ctx := context.Background()
	r := New(NewRunner(), WithPaths(paths))

	txt := `v=spf1 "quoted" \ back ; semi 'apos' include: /etc/passwd`
	desired := func(extra bool) *vrxv1.DesiredState {
		recs := []*vrxv1.DnsRecord{
			{Name: proto.String("gw.rig.example.test"), Type: proto.String("A"), Data: proto.String(fmt.Sprintf("10.%d.10.1", slot))},
			{Name: proto.String("txt.rig.example.test"), Type: proto.String("TXT"), Data: proto.String(txt)},
		}
		if extra {
			recs = append(recs, &vrxv1.DnsRecord{Name: proto.String("new.rig.example.test"), Type: proto.String("A"), Data: proto.String(fmt.Sprintf("10.%d.10.2", slot))})
		}
		res := &vrxv1.DnsResolver{
			Description:   proto.String(`rig "; rm -rf /`),
			Listen:        []*vrxv1.SocketAddress{{Address: proto.String("127.0.0.1"), Port: proto.Uint32(port)}},
			AccessControl: []*vrxv1.DnsAccessControl{{Prefix: proto.String("127.0.0.0/8"), Action: proto.String("allow")}},
			ForwardZones: []*vrxv1.DnsForwardZone{{Zone: proto.String("corp.example.test"), Forwarders: []*vrxv1.DnsUpstream{
				{Address: proto.String("192.0.2.1")}, {Address: proto.String("192.0.2.2"), Port: proto.Uint32(5353)},
			}}},
			LocalZones: []*vrxv1.DnsLocalZone{{Zone: proto.String("rig.example.test"), Records: recs}},
		}
		return &vrxv1.DesiredState{Services: &vrxv1.ServicesConfig{Dns: &vrxv1.DnsService{Resolvers: map[string]*vrxv1.DnsResolver{"rig": res}}}}
	}

	files, err := r.Render(ctx, desired(false))
	if err != nil {
		t.Fatal(err)
	}
	conf := string(files[paths.Conf()].Content)
	var ifaces []string
	for _, l := range strings.Split(conf, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), "interface: "); ok {
			ifaces = append(ifaces, v)
		}
	}
	if !slices.Equal(ifaces, []string{fmt.Sprintf("127.0.0.1@%d", port)}) {
		t.Fatalf("rendered listen list %v is not the slot's loopback port", ifaces)
	}
	t.Logf("rendered listen list: %v", ifaces)

	if err := r.Validate(ctx, files); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	t.Log("unbound-checkconf: accepted")
	bad := renderers.Files{paths.Conf(): {Mode: 0o640, Content: []byte(strings.Replace(conf, "server:\n", "server:\n\tno-such-option: yes\n", 1))}}
	if err := r.Validate(ctx, bad); !errors.Is(err, ErrDaemon) {
		t.Fatalf("broken config: want ErrDaemon, got %v", err)
	} else {
		t.Logf("unbound-checkconf rejected the broken file: %v", err)
	}

	err = r.Apply(ctx, files)
	var ar *ActionRequired
	if !errors.As(err, &ar) {
		t.Fatalf("Apply before start: want ActionRequired, got %v", err)
	}
	t.Logf("Apply before start: %v", err)

	cmd := exec.Command(unboundBin, "-d", "-c", paths.Conf()) //nolint:gosec // fixed test argv
	logf, _ := os.Create(paths.ConfDir + "/unbound.stderr")
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Logf("started pid %d: %s -d -c %s", cmd.Process.Pid, unboundBin, paths.Conf())
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		_ = logf.Close()
		t.Logf("stopped pid %d", cmd.Process.Pid)
	})
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := r.Control(ctx, "status"); err == nil {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("unbound did not come up: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	if err := r.Apply(ctx, files); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	st, err := r.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fw, _ := r.Control(ctx, "list_forwards")
	t.Logf("unbound-control list_forwards:\n%s", fw)
	found := false
	for _, z := range st.Forwards {
		if z.Zone == "corp.example.test." && slices.Contains(z.Addrs, "192.0.2.1") {
			found = true
		}
	}
	if !found {
		t.Fatalf("list_forwards lacks the rendered zone: %+v", st.Forwards)
	}

	res := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, fmt.Sprintf("127.0.0.1:%d", port))
	}}
	lctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	addrs, err := res.LookupHost(lctx, "gw.rig.example.test")
	if err != nil || !slices.Contains(addrs, fmt.Sprintf("10.%d.10.1", slot)) {
		t.Fatalf("resolve gw.rig.example.test via 127.0.0.1:%d: %v %v", port, addrs, err)
	}
	t.Logf("net.Resolver @127.0.0.1:%d gw.rig.example.test → %v", port, addrs)
	txts, err := res.LookupTXT(lctx, "txt.rig.example.test")
	if err != nil || len(txts) != 1 || txts[0] != txt {
		t.Fatalf("TXT round trip: %q %v (want %q)", txts, err, txt)
	}
	t.Logf("TXT with quotes/backslash/include: round-trips verbatim: %q", txts[0])

	p := r.NewPoller()
	evs, err := p.Poll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first poll: %v", evs)

	// change → Apply → Retrieve reflects it
	files2, err := r.Render(ctx, desired(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(ctx, files2); err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(ctx, files2); err != nil {
		t.Fatal(err)
	}
	addrs, err = res.LookupHost(lctx, "new.rig.example.test")
	if err != nil || len(addrs) != 1 {
		t.Fatalf("new record after reload: %v %v", addrs, err)
	}
	st, err = r.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(st.LocalData, func(s string) bool { return strings.HasPrefix(s, "new.rig.example.test.") }) {
		t.Fatalf("list_local_data lacks the new record: %v", st.LocalData)
	}
	t.Logf("after change: new.rig.example.test → %v; list_local_data has %d records", addrs, len(st.LocalData))
	evs, _ = p.Poll(ctx)
	t.Logf("poll after queries: %v", evs)
	msg, err := r.Retrieve(ctx)
	if err != nil || msg == nil {
		t.Fatal(err)
	}

	// rollback to the first rendering
	if err := r.Apply(ctx, files); err != nil {
		t.Fatal(err)
	}
	if _, err := res.LookupHost(lctx, "new.rig.example.test"); err == nil {
		t.Fatal("record still resolvable after rollback")
	}
	t.Log("after rollback: new.rig.example.test no longer resolves")
}
