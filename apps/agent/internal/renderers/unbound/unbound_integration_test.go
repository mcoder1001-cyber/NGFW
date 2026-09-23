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
	paths := TestPaths(prefix, slot)
	port2 := port + 1 // 3<slot>54: the listen address added below
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

	// M4: the product paths render and pass unbound-checkconf on this host (staged copy only:
	// nothing is written under /etc or /run).
	prod := New(NewRunner(), WithPaths(ProductPaths()))
	pf, err := prod.Render(ctx, &vrxv1.DnsService{Resolvers: map[string]*vrxv1.DnsResolver{"lan": {
		Listen: []*vrxv1.SocketAddress{{Address: proto.String("127.0.0.1"), Port: proto.Uint32(53)}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := prod.Validate(ctx, pf); err != nil {
		t.Fatalf("product paths: %v", err)
	}
	t.Log("product paths (control-interface /run/unbound.ctl, pidfile /run/unbound.pid, /var/lib/unbound/root.key): unbound-checkconf accepted the staged render")

	txt := `v=spf1 "quoted" \ back ; semi 'apos' include: /etc/passwd`
	var extraListen []*vrxv1.SocketAddress
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
			Listen:        append([]*vrxv1.SocketAddress{{Address: proto.String("127.0.0.1"), Port: proto.Uint32(port)}}, extraListen...),
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

	var cmd *exec.Cmd
	start := func() {
		cmd = exec.Command(unboundBin, "-d", "-c", paths.Conf()) //nolint:gosec // fixed test argv
		logf, _ := os.Create(paths.ConfDir + "/unbound.stderr")
		cmd.Stdout, cmd.Stderr = logf, logf
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Logf("started pid %d: %s -d -c %s", cmd.Process.Pid, unboundBin, paths.Conf())
		c := cmd
		t.Cleanup(func() { stopUnbound(t, c) })
		deadline := time.Now().Add(30 * time.Second)
		for {
			if _, err := r.Control(ctx, "status"); err == nil {
				return
			} else if time.Now().After(deadline) {
				t.Fatalf("unbound did not come up: %v", err)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	start()

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

	// H1 / M2: adding a listen address needs a restart; Apply must not claim success before
	// unbound listens there, and the request stays pending until unbound has restarted.
	extraListen = []*vrxv1.SocketAddress{{Address: proto.String("127.0.0.1"), Port: proto.Uint32(port2)}}
	files3, err := r.Render(ctx, desired(false))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(ctx, files3); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		err := r.Apply(ctx, files3)
		if !errors.As(err, &ar) || ar.Action != "restart" {
			t.Fatalf("listen change, Apply #%d: want restart, got %v", i, err)
		}
		t.Logf("listen change, Apply #%d: %v", i, err)
	}
	if c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port2), time.Second); err == nil {
		_ = c.Close()
		t.Fatalf("unbound already listens on %d before the restart?", port2)
	} else {
		t.Logf("before restart: 127.0.0.1:%d refused (%v) — and Apply did not report success", port2, err)
	}
	stopUnbound(t, cmd)
	start()
	if err := r.Apply(ctx, files3); err != nil {
		t.Fatalf("Apply after the restart: %v", err)
	}
	res2 := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, fmt.Sprintf("127.0.0.1:%d", port2))
	}}
	lctx2, cancel2 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel2()
	addrs, err = res2.LookupHost(lctx2, "gw.rig.example.test")
	if err != nil || len(addrs) != 1 {
		t.Fatalf("resolve via the new port %d: %v %v", port2, addrs, err)
	}
	t.Logf("after restart: Apply → nil (pending cleared, convergence check passed); net.Resolver @127.0.0.1:%d gw.rig.example.test → %v", port2, addrs)
}

func stopUnbound(t *testing.T, cmd *exec.Cmd) {
	if cmd.ProcessState != nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	t.Logf("stopped pid %d", cmd.Process.Pid)
}
