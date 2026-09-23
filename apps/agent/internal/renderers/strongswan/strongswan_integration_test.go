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
	return siteDocX(t, slot, local, remote, lts, rts, start, 30, extra)
}

func siteDocX(t *testing.T, slot int, local, remote, lts, rts, start string, dpd int, extra string) *vrxv1.DesiredState {
	t.Helper()
	js := fmt.Sprintf(`{"vpn":{"ipsec":{
	 "proposals":{"gcm":{"ike":{"encr":"aes256gcm16","prf":"prfsha256","dh":"curve25519"},"esp":{"encr":"aes256gcm16"}}},
	 "tunnels":{"w%[1]d-ab":{"ikeVersion":2,"localAddr":%[2]q,"remoteAddr":%[3]q,
	   "auth":{"method":"psk","secretRef":"psk/w%[1]d-ab"},"proposal":"gcm","localTs":[%[4]q],"remoteTs":[%[5]q],
	   "dpd":{"enabled":true,"delaySec":%[8]d,"action":"clear"},"startAction":%[6]q}%[7]s}}}}`, slot, local, remote, lts, rts, start, extra, dpd)
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
	// Validate's checker stages the files (incl. the PSKs) under TMPDIR: keep them on the
	// slot's tmpfs, never on disk (review L4).
	tmp := filepath.Join("/run/vrx-test", prefix, "tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })
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
	var pskMu sync.Mutex
	psk := integrationPSK
	resolver := strongswan.SecretResolverFunc(func(_ context.Context, ref string) ([]byte, error) {
		if ref != fmt.Sprintf("psk/w%d-ab", slot) {
			return nil, fmt.Errorf("unknown secret reference %s", ref)
		}
		pskMu.Lock()
		defer pskMu.Unlock()
		return []byte(psk), nil
	})
	daemon := strongswan.DaemonConfig{Plugins: strongswan.DefaultPlugins(), LogLevel: 1, QuietJournal: true}
	checker := strongswan.Checker{ViciSocket: h.Paths("v").ViciSocket, Runner: h.NSRunner(h.Paths("v").StrongswanConf)}
	newR := func(x string) *strongswan.Renderer {
		return strongswan.New(strongswan.WithPaths(h.Paths(x)), strongswan.WithSecretResolver(resolver),
			strongswan.WithDaemonConfig(daemon), strongswan.WithChecker(checker), strongswan.WithLogger(logger),
			strongswan.WithOwnerPrefix(prefix))
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

	// ---- review M1: a soft edit (DPD 30→20) plus start_action none→start keeps the IKE_SA and
	// does not add a second CHILD_SA.
	stateOf := func(r *strongswan.Renderer) *strongswan.State {
		t.Helper()
		st, err := r.State(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	waitFor := func(what string, cond func() bool) {
		t.Helper()
		dl := time.Now().Add(20 * time.Second)
		for !cond() {
			if time.Now().After(dl) {
				t.Fatalf("timeout waiting for %s: a %+v b %+v", what, stateOf(ra).SAs, stateOf(rb).SAs)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	render := func(r *strongswan.Renderer, ds *vrxv1.DesiredState) renderers.Files {
		t.Helper()
		f, err := r.Render(ctx, ds)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Validate(ctx, f); err != nil {
			t.Fatal(err)
		}
		return f
	}
	curA := render(ra, siteDocX(t, slot, aAddr, bAddr, lanA, lanB, "start", 20, ""))
	impact, err := ra.Impact(curA)
	if err != nil || impact[conn] != "update" {
		t.Fatalf("Impact(soft edit) = %v %v", impact, err)
	}
	if err := ra.Apply(ctx, curA); err != nil {
		t.Fatalf("Apply soft edit: %v", err)
	}
	st = stateOf(ra)
	if len(st.SAs) != 1 || st.SAs[0].UniqueID != sa.UniqueID || len(st.SAs[0].Children) != 1 {
		t.Fatalf("soft edit disturbed the tunnel: %+v", st.SAs)
	}
	t.Logf("M1: Impact %v; Apply(dpd 30→20, start_action none→start): IKE_SA #%s kept, %d CHILD_SA (no duplicate)", impact, sa.UniqueID, len(st.SAs[0].Children))

	// ---- review H1 probe 1: the responder b narrows its remote selector to 10.<N>.1.0/25. The
	// old /24 CHILD_SA must be gone before Apply returns; a follows, and the renderer (start
	// action) brings up a CHILD_SA that carries the /25.
	narrow := fmt.Sprintf("10.%d.1.0/25", slot)
	curB := render(rb, siteDocX(t, slot, bAddr, aAddr, lanB, narrow, "none", 30, ""))
	if im, _ := rb.Impact(curB); im[conn] != "reestablish" {
		t.Fatalf("Impact(narrowed) = %v", im)
	}
	if err := rb.Apply(ctx, curB); err != nil {
		t.Fatalf("Apply b narrowed: %v", err)
	}
	stB := stateOf(rb)
	for _, s := range stB.SAs {
		for _, c := range s.Children {
			if c.State == "INSTALLED" && !slices.Equal(c.RemoteTS, []string{narrow}) {
				t.Fatalf("b still has CHILD_SA #%s with remote TS %v after Apply", c.UniqueID, c.RemoteTS)
			}
		}
	}
	xp, _ := h.XfrmPolicy(ctx, h.NetNS("b"))
	if strings.Contains(xp, "src "+lanA) || strings.Contains(xp, "dst "+lanA) {
		t.Fatalf("b xfrm policy still covers %s:\n%s", lanA, xp)
	}
	t.Logf("H1 probe 1: Apply(b, remote_ts %s) returned after terminating the /24 CHILD_SA; b: stale %d, xfrm policy has no %s", narrow, stB.StaleSAs, lanA)
	curA = render(ra, siteDocX(t, slot, aAddr, bAddr, narrow, lanB, "start", 20, ""))
	if err := ra.Apply(ctx, curA); err != nil {
		t.Fatalf("Apply a narrowed: %v", err)
	}
	waitFor("a /25 CHILD_SA on both sides", func() bool {
		for _, r := range []*strongswan.Renderer{ra, rb} {
			st := stateOf(r)
			if len(st.SAs) != 1 || len(st.SAs[0].Children) != 1 || st.SAs[0].Children[0].State != "INSTALLED" || st.StaleSAs != 0 {
				return false
			}
		}
		c := stateOf(ra).SAs[0].Children[0]
		return slices.Equal(c.LocalTS, []string{narrow})
	})
	ca := stateOf(ra).SAs[0]
	t.Logf("H1 probe 1: a followed; IKE_SA #%s CHILD_SA #%s INSTALLED local_ts %v remote_ts %v (b: remote %v)",
		ca.UniqueID, ca.Children[0].UniqueID, ca.Children[0].LocalTS, ca.Children[0].RemoteTS, stateOf(rb).SAs[0].Children[0].RemoteTS)

	// ---- review H1 probe 2: rotate the PSK on both sides. The IKE_SA authenticated with the old
	// key must be gone; the new one is authenticated with the new key (charon would fail AUTH
	// otherwise).
	oldIKE := ca.UniqueID
	pskMu.Lock()
	psk = integrationPSK + "_rotated"
	pskMu.Unlock()
	curB = render(rb, siteDocX(t, slot, bAddr, aAddr, lanB, narrow, "none", 30, ""))
	if err := rb.Apply(ctx, curB); err != nil {
		t.Fatalf("Apply b rotated: %v", err)
	}
	curA = render(ra, siteDocX(t, slot, aAddr, bAddr, narrow, lanB, "start", 20, ""))
	if im, _ := ra.Impact(curA); im[conn] != "reestablish" {
		t.Fatalf("Impact(rotated PSK) = %v", im)
	}
	if err := ra.Apply(ctx, curA); err != nil {
		t.Fatalf("Apply a rotated: %v", err)
	}
	waitFor("a new IKE_SA with the rotated PSK", func() bool {
		sa, sb := stateOf(ra).SAs, stateOf(rb).SAs
		return len(sa) == 1 && sa[0].UniqueID != oldIKE && sa[0].State == "ESTABLISHED" && len(sa[0].Children) == 1 &&
			len(sb) == 1 && sb[0].State == "ESTABLISHED"
	})
	t.Logf("H1 probe 2: PSK rotated on b then a: old IKE_SA #%s gone, IKE_SA #%s ESTABLISHED (authenticated with the new key)", oldIKE, stateOf(ra).SAs[0].UniqueID)

	// ---- review M3: charon a crashes (SIGKILL to the PID the harness spawned). Its xfrm states
	// stay; the restarted charon is reported as restarted (State + event) until acknowledged.
	if err := h.Crash("a"); err != nil {
		t.Fatal(err)
	}
	residue, _ := h.XfrmState(ctx, h.NetNS("a"))
	if strings.Count(residue, "proto esp") == 0 {
		t.Fatalf("expected xfrm residue after the crash, got %q", residue)
	}
	if _, err := h.Start(ctx, "a", h.NetNS("a"), filesA[h.Paths("a").StrongswanConf].Content); err != nil {
		t.Fatal(err)
	}
	st = stateOf(ra)
	if !st.Restarted || len(st.Conns) != 0 {
		t.Fatalf("restart not reported: %+v", st)
	}
	waitFor("the restarted event", func() bool {
		gotMu.Lock()
		defer gotMu.Unlock()
		return slices.ContainsFunc(got, func(e strongswan.Event) bool { return e.Kind == strongswan.KindDaemon && e.State == "restarted" })
	})
	t.Logf("M3: after SIGKILL + restart: State.restarted=%v (started %q, acked %q), Watch sent daemon/restarted; xfrm residue in %s:\n%s",
		st.Restarted, st.DaemonStartedAt, st.AckedStartedAt, h.NetNS("a"), residue)
	if err := h.FlushXfrm(ctx, h.NetNS("a")); err != nil {
		t.Fatal(err)
	}
	if err := ra.AckRestart(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ra.Apply(ctx, curA); err != nil {
		t.Fatalf("Apply after restart: %v", err)
	}
	waitFor("a re-established after the restart", func() bool {
		sa := stateOf(ra).SAs
		return len(sa) == 1 && sa[0].State == "ESTABLISHED" && len(sa[0].Children) == 1
	})
	st = stateOf(ra)
	t.Logf("M3: residue flushed, AckRestart → restarted=%v; re-Apply loaded %s and initiated it (start action): IKE_SA #%s ESTABLISHED", st.Restarted, conn, st.SAs[0].UniqueID)
	for !eventsSeen(strongswan.KindDaemon, false) || !eventsSeen(strongswan.KindDaemon, true) {
		if time.Now().After(deadline.Add(60 * time.Second)) {
			t.Fatalf("Watch did not report the restart: %v", got)
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Log("events: Watch reported daemon down and, after re-subscribing, daemon restarted")

	// Terminate → SAs gone on both sides.
	if err := ra.Terminate(ctx, conn, 5000); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	// b may still hold the SA of the crashed charon (it only learns by DPD): terminate there too.
	if st := stateOf(rb); len(st.SAs) > 0 {
		if err := rb.Terminate(ctx, conn, 5000); err != nil {
			t.Fatalf("terminate b: %v", err)
		}
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
	rotated := integrationPSK + "_rotated"
	forms := []string{integrationPSK, base64.StdEncoding.EncodeToString([]byte(integrationPSK)), fmt.Sprintf("%x", integrationPSK),
		base64.StdEncoding.EncodeToString([]byte(rotated))}
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
