package strongswan_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/strongswan"
	"ngfw/agent/internal/renderers/strongswan/swantest"
	"ngfw/agent/internal/vpp/vpptest"
)

// integrationPSK is the fixture PSK (00-CONTEXT: VRX_TEST_PSK_<id>); the test proves it never
// leaves the 0600 secrets file and charon's memory.
const integrationPSK = "VRX_TEST_PSK_RF2_integration"

// siteDoc renders one side of the w<N>-ab tunnel.
func siteDoc(t *testing.T, slot int, local, remote, lts, rts, start string, extra string) *vrxv1.DesiredState {
	t.Helper()
	js := fmt.Sprintf(`{"vpn":{"ipsec":{
	 "proposals":{"gcm":{"ike":{"encr":"aes256gcm16","prf":"prfsha256","dh":"curve25519"},"esp":{"encr":"aes256gcm16"}}},
	 "tunnels":{"w%[1]d-ab":{"ikeVersion":2,"localAddr":%[2]q,"remoteAddr":%[3]q,
	   "auth":{"method":"psk","secretRef":"psk/w%[1]d-ab"},"proposal":"gcm","localTs":[%[4]q],"remoteTs":[%[5]q],
	   "dpd":{"enabled":true,"delaySec":30,"action":"clear"},"startAction":%[6]q}%[7]s}}}}`, slot, local, remote, lts, rts, start, extra)
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatalf("fixture: %v\n%s", err, js)
	}
	return ds
}

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func statEtc(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, p := range []string{"/etc/strongswan.conf", "/etc/swanctl", "/etc/strongswan.d", "/usr/lib/ipsec"} {
		info, err := os.Stat(p)
		if err != nil {
			fmt.Fprintf(&b, "%s: absent; ", p)
			continue
		}
		fmt.Fprintf(&b, "%s: %v %d %s; ", p, info.Mode(), info.Size(), info.ModTime().UTC().Format(time.RFC3339Nano))
	}
	return b.String()
}

// TestStrongswanIntegration: two test charons in ns-<prefix>-a / ns-<prefix>-b (veth
// <prefix>-a ↔ <prefix>-b, 10.<N>.250.1/2) and a scratch charon in ns-<prefix>-v for the
// Validate checker. Render → Validate (strict + swanctl --load-all into the scratch charon) →
// Apply (VICI) → initiate → ESTABLISHED on both sides, xfrm states (kernel-netlink, no VPP) →
// idempotent re-apply → failed Apply rolls back → charon restart + re-apply → terminate →
// unload → Retrieve empty, xfrm empty. Throughout: the PSK appears nowhere but the 0600 file.
func TestStrongswanIntegration(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	prefix, slot := vpptest.Prefix(t), vpptest.Slot(t)
	root := swantest.FindRoot(prefix)
	if root == "" {
		t.Skipf("no strongSwan binaries: install strongSwan or extract the stock packages into %s (README.md)", swantest.StockRoot(prefix))
	}
	etcBefore := statEtc(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	h, err := swantest.New(prefix, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Errorf("harness close: %v", err)
		}
		if left := h.Running(); len(left) > 0 {
			t.Errorf("test daemons still running: %v", left)
		}
		if root != "/" {
			if _, err := os.Stat("/usr/lib/ipsec"); err == nil {
				t.Error("the stock overlay leaked into the host mount namespace")
			}
		}
		if after := statEtc(t); after != etcBefore {
			t.Errorf("/etc strongSwan paths changed:\nbefore %s\nafter  %s", etcBefore, after)
		}
		t.Logf("cleanup: daemons stopped (none left under %s), namespaces deleted; /etc unchanged: %s", h.Base, etcBefore)
	})
	t.Logf("strongSwan binaries: root %s (charon-systemd, swanctl)", root)

	for _, x := range []string{"a", "b", "v"} {
		if _, err := h.AddNetNS(ctx, x); err != nil {
			t.Fatal(err)
		}
	}
	rig := netip.MustParsePrefix(fmt.Sprintf("10.%d.0.0/16", slot))
	aAddr, bAddr := fmt.Sprintf("10.%d.250.1", slot), fmt.Sprintf("10.%d.250.2", slot)
	if err := h.Link(ctx, "a", aAddr+"/24", "b", bAddr+"/24"); err != nil {
		t.Fatal(err)
	}

	var logs syncBuffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	resolver := strongswan.SecretResolverFunc(func(_ context.Context, ref string) ([]byte, error) {
		if ref != fmt.Sprintf("psk/w%d-ab", slot) {
			return nil, fmt.Errorf("unknown secret reference %s", ref)
		}
		return []byte(integrationPSK), nil
	})
	daemon := strongswan.DaemonConfig{Plugins: strongswan.DefaultPlugins(), LogLevel: 1, QuietJournal: true}
	checker := strongswan.Checker{ViciSocket: h.Paths("v").ViciSocket, Runner: h.NSRunner(h.Paths("v").StrongswanConf)}
	newR := func(x string) *strongswan.Renderer {
		return strongswan.New(strongswan.WithPaths(h.Paths(x)), strongswan.WithSecretResolver(resolver),
			strongswan.WithDaemonConfig(daemon), strongswan.WithChecker(checker), strongswan.WithLogger(logger))
	}
	ra, rb, rv := newR("a"), newR("b"), strongswan.New(strongswan.WithPaths(h.Paths("v")), strongswan.WithDaemonConfig(daemon))

	lanA, lanB := fmt.Sprintf("10.%d.1.0/24", slot), fmt.Sprintf("10.%d.2.0/24", slot)
	docA := siteDoc(t, slot, aAddr, bAddr, lanA, lanB, "none", "")
	docB := siteDoc(t, slot, bAddr, aAddr, lanB, lanA, "none", "")
	filesA, err := ra.Render(ctx, docA)
	if err != nil {
		t.Fatal(err)
	}
	filesB, err := rb.Render(ctx, docB)
	if err != nil {
		t.Fatal(err)
	}
	filesV, err := rv.Render(ctx, &vrxv1.DesiredState{})
	if err != nil {
		t.Fatal(err)
	}
	// Assert the rendered endpoints are rig addresses before anything binds.
	for name, files := range map[string]renderers.Files{"a": filesA, "b": filesB} {
		tree, err := strongswan.ParseSettings("vrx.conf", files[h.Paths(name).ConnsFile()].Content)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range tree.Sub("connections").Sections() {
			for _, k := range []string{"local_addrs", "remote_addrs"} {
				v, _ := c.Section.Get(k)
				if a, err := netip.ParseAddr(v); err != nil || !rig.Contains(a) {
					t.Fatalf("%s: rendered %s = %q is not in the rig range %s", name, k, v, rig)
				}
			}
		}
	}
	t.Logf("rendered endpoints are in %s; IKE binds only inside %s / %s", rig, h.NetNS("a"), h.NetNS("b"))

	for x, files := range map[string]renderers.Files{"a": filesA, "b": filesB, "v": filesV} {
		d, err := h.Start(ctx, x, h.NetNS(x), files[h.Paths(x).StrongswanConf].Content)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("charon %s started in %s, VICI %s", d.Name, d.NetNS, d.Paths.ViciSocket)
	}
	t.Logf("test daemons (argv0 under %s): pids %v", h.Base, h.Running())

	// Validate: strict round trip + swanctl --load-all into the scratch charon v.
	for name, pair := range map[string]struct {
		r     *strongswan.Renderer
		files renderers.Files
	}{"a": {ra, filesA}, "b": {rb, filesB}} {
		if err := pair.r.Validate(ctx, pair.files); err != nil {
			t.Fatalf("Validate %s: %v", name, err)
		}
	}
	stV, err := rv.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stV.Conns) != 1 || len(stV.SAs) != 0 {
		t.Fatalf("checker charon: %d conns %d SAs, want the checked conn loaded and nothing initiated", len(stV.Conns), len(stV.SAs))
	}
	t.Logf("Validate a+b: strict parser ok; swanctl --load-all into scratch charon v ok (v has %s, start_action none, 0 SAs)", stV.Conns[0].Name)

	// Events from a.
	events := make(chan strongswan.Event, 128)
	wctx, wcancel := context.WithCancel(ctx)
	watchDone := make(chan error, 1)
	go func() { watchDone <- ra.Watch(wctx, events) }()
	var got []strongswan.Event
	var gotMu sync.Mutex
	collectDone := make(chan struct{})
	go func() {
		defer close(collectDone)
		for e := range events {
			gotMu.Lock()
			got = append(got, e)
			gotMu.Unlock()
		}
	}()
	eventsSeen := func(kind string, up bool) bool {
		gotMu.Lock()
		defer gotMu.Unlock()
		return slices.ContainsFunc(got, func(e strongswan.Event) bool { return e.Kind == kind && e.Up == up })
	}

	// Apply via VICI on both sides.
	if err := ra.Apply(ctx, filesA); err != nil {
		t.Fatalf("Apply a: %v", err)
	}
	if err := rb.Apply(ctx, filesB); err != nil {
		t.Fatalf("Apply b: %v", err)
	}
	info, err := os.Stat(h.Paths("a").SecretsFile())
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("secrets file mode %v %v", info.Mode(), err)
	}
	conn := fmt.Sprintf("w%d-ab", slot)
	st, err := ra.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Conns) != 1 || st.Conns[0].Name != conn || len(st.SharedSecrets) != 1 {
		t.Fatalf("after Apply: %+v", st)
	}
	t.Logf("Apply a+b ok: list-conns %s %s local %v remote %v children %v; get-shared %v; secrets file mode %v",
		st.Conns[0].Name, st.Conns[0].Version, st.Conns[0].LocalAddrs, st.Conns[0].RemoteAddrs, st.Conns[0].Children, st.SharedSecrets, info.Mode().Perm())

	// Initiate from a; ESTABLISHED on both sides within 10 s.
	if err := ra.Initiate(ctx, conn, conn, 10000); err != nil {
		t.Fatalf("initiate: %v", err)
	}
	var sa, saB strongswan.IKESA
	deadline := time.Now().Add(10 * time.Second)
	for {
		stA, errA := ra.State(ctx)
		stB, errB := rb.State(ctx)
		if errA == nil && errB == nil && len(stA.SAs) == 1 && len(stB.SAs) == 1 &&
			stA.SAs[0].State == "ESTABLISHED" && stB.SAs[0].State == "ESTABLISHED" &&
			len(stA.SAs[0].Children) == 1 && stA.SAs[0].Children[0].State == "INSTALLED" {
			sa, saB = stA.SAs[0], stB.SAs[0]
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no ESTABLISHED SA within 10 s: a %+v (%v) b %+v (%v)", stA, errA, stB, errB)
		}
		time.Sleep(200 * time.Millisecond)
	}
	excerpt, _ := json.MarshalIndent(map[string]any{"a": sa, "b.state": saB.State, "b.remoteHost": saB.RemoteHost}, "", "  ")
	t.Logf("list-sas (a, via Retrieve; PSK never present):\n%s", excerpt)
	xs, err := h.XfrmState(ctx, h.NetNS("a"))
	if err != nil || strings.Count(xs, "proto esp") != 2 {
		t.Fatalf("xfrm state in %s: %q %v (want 2 ESP states)", h.NetNS("a"), xs, err)
	}
	t.Logf("ip -n %s xfrm state (keys removed):\n%s", h.NetNS("a"), xs)
	for !eventsSeen("ike-updown", true) || !eventsSeen("child-updown", true) {
		if time.Now().After(deadline.Add(5 * time.Second)) {
			t.Fatalf("no up events: %v", got)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Log("events: ike-updown up + child-updown up received from VICI")

	// Idempotent re-apply: the SA survives (same unique id).
	if err := ra.Apply(ctx, filesA); err != nil {
		t.Fatalf("re-Apply: %v", err)
	}
	st, err = ra.State(ctx)
	if err != nil || len(st.SAs) != 1 || st.SAs[0].UniqueID != sa.UniqueID || st.SAs[0].State != "ESTABLISHED" {
		t.Fatalf("re-apply disturbed the SA: %+v %v", st, err)
	}
	t.Logf("idempotent re-Apply: IKE_SA #%s still ESTABLISHED", sa.UniqueID)

	// A failing Apply rolls back: a second tunnel whose certificate file charon cannot parse.
	if err := os.WriteFile(filepath.Join(h.Paths("a").SwanctlDir, "x509", "w-bad.pem"), []byte("not a certificate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	badDoc := siteDoc(t, slot, aAddr, bAddr, lanA, lanB, "none", fmt.Sprintf(`,"w%d-bad":{"localAddr":%q,"remoteAddr":"10.%d.250.9",
	  "auth":{"method":"cert","certificate":"w-bad"},"proposal":"gcm","localTs":[%q],"remoteTs":["10.%d.9.0/24"]}`, slot, aAddr, slot, lanA, slot))
	badFiles, err := ra.Render(ctx, badDoc)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(h.Paths("a").ConnsFile())
	applyErr := ra.Apply(ctx, badFiles)
	if applyErr == nil {
		t.Fatal("Apply with an unparsable certificate succeeded")
	}
	after, _ := os.ReadFile(h.Paths("a").ConnsFile())
	st, err = ra.State(ctx)
	if err != nil || !bytes.Equal(before, after) || len(st.Conns) != 1 || len(st.SAs) != 1 || st.SAs[0].UniqueID != sa.UniqueID {
		t.Fatalf("rollback incomplete: files equal %v, state %+v, %v", bytes.Equal(before, after), st, err)
	}
	t.Logf("failed Apply rolled back: error %q; vrx.conf restored byte-for-byte; charon still has only %s with IKE_SA #%s", applyErr, conn, sa.UniqueID)

	// Restart safety: charon a restarts empty; re-Apply of the files restores the config and
	// the tunnel comes back (b still has its SA until DPD; initiate again from a).
	if err := h.Stop("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Start(ctx, "a", h.NetNS("a"), filesA[h.Paths("a").StrongswanConf].Content); err != nil {
		t.Fatal(err)
	}
	if st, err := ra.State(ctx); err != nil || len(st.Conns) != 0 {
		t.Fatalf("restarted charon not empty: %+v %v", st, err)
	}
	if err := ra.Apply(ctx, filesA); err != nil {
		t.Fatalf("Apply after restart: %v", err)
	}
	if err := ra.Initiate(ctx, conn, conn, 10000); err != nil {
		t.Fatalf("initiate after restart: %v", err)
	}
	if st, err = ra.State(ctx); err != nil || len(st.SAs) != 1 || st.SAs[0].State != "ESTABLISHED" {
		t.Fatalf("after restart: %+v %v", st, err)
	}
	t.Logf("charon a restarted (SIGTERM to the PID the harness spawned, then started again): re-Apply loaded %s from the files, IKE_SA #%s ESTABLISHED", conn, st.SAs[0].UniqueID)
	for !eventsSeen(strongswan.KindDaemon, false) || !eventsSeen(strongswan.KindDaemon, true) {
		if time.Now().After(deadline.Add(60 * time.Second)) {
			t.Fatalf("Watch did not report the restart: %v", got)
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Log("events: Watch reported daemon down and, after re-subscribing, daemon up")

	// Terminate → SAs gone on both sides.
	if err := ra.Terminate(ctx, conn, 5000); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	for {
		stA, _ := ra.State(ctx)
		stB, _ := rb.State(ctx)
		if stA != nil && stB != nil && len(stA.SAs) == 0 && len(stB.SAs) == 0 {
			break
		}
		if time.Now().After(deadline.Add(90 * time.Second)) {
			t.Fatalf("SAs still up after terminate: %+v / %+v", stA, stB)
		}
		time.Sleep(200 * time.Millisecond)
	}
	// Unload (apply the empty document) → Retrieve empty, xfrm empty.
	for x, r := range map[string]*strongswan.Renderer{"a": ra, "b": rb} {
		empty, err := r.Render(ctx, &vrxv1.DesiredState{})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Apply(ctx, empty); err != nil {
			t.Fatalf("Apply empty %s: %v", x, err)
		}
		st, err := r.State(ctx)
		if err != nil || len(st.Conns) != 0 || len(st.SAs) != 0 || len(st.SharedSecrets) != 0 {
			t.Fatalf("%s not empty after unload: %+v %v", x, st, err)
		}
		xs, err := h.XfrmState(ctx, h.NetNS(x))
		if err != nil || xs != "" {
			t.Fatalf("xfrm state in %s after unload: %q %v", h.NetNS(x), xs, err)
		}
		xp, err := h.XfrmPolicy(ctx, h.NetNS(x))
		if err != nil || strings.Contains(xp, "tmpl") {
			t.Fatalf("xfrm policy in %s after unload: %q %v", h.NetNS(x), xp, err)
		}
	}
	pb, err := ra.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	retrieved, _ := protojson.Marshal(pb)
	t.Logf("after terminate + unload: Retrieve a = conns [] sas [] sharedSecrets []; xfrm state/policy empty in both namespaces")
	for !eventsSeen("ike-updown", false) {
		if time.Now().After(deadline.Add(100 * time.Second)) {
			t.Fatalf("no down event: %v", got)
		}
		time.Sleep(100 * time.Millisecond)
	}
	wcancel()
	<-watchDone
	close(events)
	<-collectDone

	// Planted secret: the PSK (and its base64/hex forms) appears nowhere except the 0600
	// secrets file — not in vrx.conf, strongswan.conf, charon's logs, the renderer log,
	// Retrieve, events or errors.
	forms := []string{integrationPSK, base64.StdEncoding.EncodeToString([]byte(integrationPSK)), fmt.Sprintf("%x", integrationPSK)}
	sources := map[string]string{"renderer log": logs.String(), "retrieve": string(retrieved), "rollback error": applyErr.Error()}
	for _, x := range []string{"a", "b"} {
		for _, p := range []string{h.Paths(x).ConnsFile(), h.Paths(x).StrongswanConf, h.Paths(x).LogFile} {
			b, err := os.ReadFile(p) //nolint:gosec // test paths
			if err != nil {
				t.Fatal(err)
			}
			sources[p] = string(b)
		}
	}
	gotMu.Lock()
	for _, e := range got {
		sources["event "+e.String()] = protojson.Format(e.ToProto())
	}
	gotMu.Unlock()
	for name, text := range sources {
		for _, f := range forms {
			if strings.Contains(text, f) {
				t.Errorf("PSK (form %.6s…) found in %s", f, name)
			}
		}
	}
	t.Logf("planted secret: not found in %d sources (vrx.conf, strongswan.conf, charon logs a/b, renderer log, Retrieve, %d events, errors)", len(sources), len(got))
	if testing.Verbose() {
		fmt.Printf("RF2_EVIDENCE_LOGS=%s\n", h.Base)
	}
}
