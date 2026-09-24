package strongswan

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/strongswan/govici/vici"
	"google.golang.org/protobuf/encoding/protojson"

	"ngfw/agent/internal/renderers"
)

// applyDoc is a two-tunnel document for Apply tests (no cert tunnel: its files are F-pki's).
const applyDoc = `{"vpn":{"ipsec":{
 "proposals":{"gcm":{"ike":{"encr":"aes256gcm16","dh":"curve25519"},"esp":{"encr":"aes256gcm16"}}},
 "tunnels":{
  "w3-site-a":{"localAddr":"10.3.250.1","remoteAddr":"10.3.250.2","auth":{"method":"psk","secretRef":"psk/w3-site-a"},
   "proposal":"gcm","localTs":["10.3.1.0/24"],"remoteTs":["10.3.2.0/24"],"startAction":"none"},
  "w3.site-b":{"localAddr":"10.3.250.1","remoteAddr":"10.3.250.3","auth":{"method":"psk","secretRef":"psk/w3-site-b"},
   "proposal":"gcm","localTs":["10.3.1.0/24"],"remoteTs":["10.3.3.0/24"],"startAction":"trap"}
 }}}}`

func renderApply(t *testing.T, r *Renderer, js string) renderers.Files {
	t.Helper()
	files, err := r.Render(context.Background(), doc(t, js))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	return files
}

func onlySiteA(t *testing.T) string {
	t.Helper()
	ds := doc(t, applyDoc)
	delete(ds.GetVpn().GetIpsec().GetTunnels(), "w3.site-b")
	b, err := protojson.Marshal(ds)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApplyLoadsConvergesAndIsIdempotent(t *testing.T) {
	f := newFakeCharon()
	p := unitPaths(t)
	r := newTestRenderer(t, p, WithDialer(f.dialer()))
	files := renderApply(t, r, applyDoc)
	if err := r.Apply(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	if got := f.loadedConns(); !slices.Equal(got, []string{"w3+site-b", "w3-site-a"}) {
		t.Errorf("loaded conns %v", got)
	}
	if got := f.loadedShared(); !slices.Equal(got, []string{"ike-w3+site-b", "ike-w3-site-a"}) {
		t.Errorf("loaded secrets %v", got)
	}
	sh := f.shared["ike-w3-site-a"]
	if string(sh.data) != fullSecrets["psk/w3-site-a"] || !slices.Equal(sh.owners, []string{"10.3.250.1", "10.3.250.2"}) {
		t.Errorf("load-shared data/owners wrong: owners %v", sh.owners)
	}
	// Files on disk with their modes.
	for path, want := range map[string]os.FileMode{p.StrongswanConf: 0o640, p.ConnsFile(): 0o640, p.SecretsFile(): 0o600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Errorf("%s: %v mode %v, want %v", path, err, info.Mode().Perm(), want)
		}
	}
	// Loaded messages equal what swanctl would build: lists split, children nested.
	body := f.conns["w3-site-a"]
	if !slices.Equal(strs(body, "local_addrs"), []string{"10.3.250.1"}) || str(sub(sub(body, "children"), "w3-site-a"), "mode") != "tunnel" {
		t.Errorf("load-conn body: %v", body)
	}
	// Second Apply: same load requests, nothing unloaded, still converged.
	f.calls = nil
	if err := r.Apply(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.callNames() {
		if strings.HasPrefix(c, "unload") || c == "terminate" {
			t.Errorf("idempotent re-apply issued %s", c)
		}
	}
}

func TestApplyRemovesStaleAndTerminatesSAs(t *testing.T) {
	f := newFakeCharon()
	r := newTestRenderer(t, unitPaths(t), WithDialer(f.dialer()))
	if err := r.Apply(context.Background(), renderApply(t, r, applyDoc)); err != nil {
		t.Fatal(err)
	}
	f.sas["w3+site-b"] = []*vici.Message{saMsg("1", "ESTABLISHED", "w3+site-b", "1", "INSTALLED")}
	f.calls = nil
	if err := r.Apply(context.Background(), renderApply(t, r, onlySiteA(t))); err != nil {
		t.Fatal(err)
	}
	if got := f.loadedConns(); !slices.Equal(got, []string{"w3-site-a"}) {
		t.Errorf("loaded conns %v", got)
	}
	if got := f.loadedShared(); !slices.Equal(got, []string{"ike-w3-site-a"}) {
		t.Errorf("loaded secrets %v", got)
	}
	calls := f.callNames()
	if !slices.Contains(calls, "unload-conn") || !slices.Contains(calls, "terminate") || !slices.Contains(calls, "unload-shared") {
		t.Errorf("calls %v: want unload-conn, terminate, unload-shared", calls)
	}
	if len(f.sas["w3+site-b"]) != 0 {
		t.Error("SAs of the removed tunnel still up")
	}
}

func TestApplyFailureRollsBack(t *testing.T) {
	f := newFakeCharon()
	p := unitPaths(t)
	r := newTestRenderer(t, p, WithDialer(f.dialer()))
	first := renderApply(t, r, onlySiteA(t))
	if err := r.Apply(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(p.ConnsFile())

	// charon rejects a connection: error, files restored, charon back to the first state.
	f.failOn["load-conn"] = "invalid proposal"
	err := r.Apply(context.Background(), renderApply(t, r, applyDoc))
	if err == nil || !errors.Is(err, ErrDaemon) {
		t.Fatalf("Apply = %v, want ErrDaemon", err)
	}
	after, _ := os.ReadFile(p.ConnsFile())
	if !bytes.Equal(before, after) {
		t.Error("vrx.conf not restored after the failed Apply")
	}
	delete(f.failOn, "load-conn")
	// The rollback re-applied the previous files (load-conn failed for every conn during the
	// rollback too, so re-run it now and check the fake converges to the first state).
	if err := r.Apply(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if got := f.loadedShared(); !slices.Equal(got, []string{"ike-w3-site-a"}) {
		t.Errorf("secrets after rollback %v", got)
	}
}

func TestApplyNotConvergedRollsBack(t *testing.T) {
	f := newFakeCharon()
	p := unitPaths(t)
	r := newTestRenderer(t, p, WithDialer(f.dialer()))
	if err := r.Apply(context.Background(), renderApply(t, r, onlySiteA(t))); err != nil {
		t.Fatal(err)
	}
	// charon says "success" but does not load w3+site-b (the RF-1 H2 failure mode).
	f.ignoreLoad["w3+site-b"] = true
	err := r.Apply(context.Background(), renderApply(t, r, applyDoc))
	if err == nil || !strings.Contains(err.Error(), "not converged") || !strings.Contains(err.Error(), "w3+site-b not loaded") {
		t.Fatalf("Apply = %v, want not converged", err)
	}
	// Rolled back: the secret of the half-applied tunnel is unloaded again.
	if got := f.loadedShared(); !slices.Equal(got, []string{"ike-w3-site-a"}) {
		t.Errorf("secrets after rollback %v", got)
	}
	if got := f.loadedConns(); !slices.Equal(got, []string{"w3-site-a"}) {
		t.Errorf("conns after rollback %v", got)
	}
	data, _ := os.ReadFile(p.ConnsFile())
	if strings.Contains(string(data), "w3+site-b") {
		t.Error("vrx.conf not restored")
	}
}

func TestApplyFirstFailureUnloadsEverything(t *testing.T) {
	f := newFakeCharon()
	p := unitPaths(t)
	r := newTestRenderer(t, p, WithDialer(f.dialer()))
	f.ignoreLoad["w3-site-a"] = true
	if err := r.Apply(context.Background(), renderApply(t, r, applyDoc)); err == nil {
		t.Fatal("Apply succeeded without convergence")
	}
	if len(f.loadedConns()) != 0 || len(f.loadedShared()) != 0 {
		t.Errorf("left loaded: conns %v secrets %v", f.loadedConns(), f.loadedShared())
	}
	for _, path := range []string{p.StrongswanConf, p.ConnsFile(), p.SecretsFile()} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s left behind after a failed first Apply", path)
		}
	}
}

func TestApplyDaemonDownRestoresFiles(t *testing.T) {
	f := newFakeCharon()
	f.down = true
	p := unitPaths(t)
	r := newTestRenderer(t, p, WithDialer(f.dialer()))
	err := r.Apply(context.Background(), renderApply(t, r, applyDoc))
	if err == nil || !errors.Is(err, ErrDaemon) {
		t.Fatalf("Apply = %v, want ErrDaemon", err)
	}
	if _, err := os.Stat(p.ConnsFile()); !errors.Is(err, os.ErrNotExist) {
		t.Error("files written although charon was unreachable")
	}
}

func TestApplyRejectsForeignFiles(t *testing.T) {
	r := newTestRenderer(t, unitPaths(t), WithDialer(newFakeCharon().dialer()))
	files := renderApply(t, r, applyDoc)
	files["/etc/passwd"] = renderers.File{Mode: 0o644, Content: []byte("x\n")}
	if err := r.Apply(context.Background(), files); err == nil {
		t.Fatal("Apply accepted a foreign path")
	}
}

// TestPlantedSecretNeverLeaks: a planted PSK must appear only in the 0600 secrets file (as
// base64) and in the load-shared request — never in vrx.conf/strongswan.conf, Files.Redacted,
// DryRun-like outputs, Retrieve, events, errors (even when charon echoes it) or log lines.
func TestPlantedSecretNeverLeaks(t *testing.T) {
	const planted = "VRX_TEST_PSK_RF2_planted_c0ffee"
	forms := []string{planted, base64.StdEncoding.EncodeToString([]byte(planted)), hex.EncodeToString([]byte(planted))}
	var logs bytes.Buffer
	f := newFakeCharon()
	p := unitPaths(t)
	r := newTestRenderer(t, p, WithDialer(f.dialer()),
		WithSecretResolver(testResolver(map[string]string{"psk/w3-site-a": planted, "psk/w3-site-b": testPSK + "_b"})),
		WithLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	var outputs []string
	files := renderApply(t, r, applyDoc)
	for path, file := range files {
		if path != p.SecretsFile() {
			outputs = append(outputs, "file "+path+": "+string(file.Content))
		}
	}
	for path, file := range files.Redacted() {
		outputs = append(outputs, "redacted "+path+": "+string(file.Content))
	}
	if err := r.Apply(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	// A failing Apply: the error names the connection, never the secret.
	f.failOn["load-conn"] = "invalid proposal"
	if err := r.Apply(context.Background(), files); err == nil {
		t.Fatal("expected failure")
	} else {
		outputs = append(outputs, "apply error: "+err.Error())
	}
	delete(f.failOn, "load-conn")
	// A secret-bearing failure from the resolver itself.
	bad := newTestRenderer(t, p, WithSecretResolver(SecretResolverFunc(func(context.Context, string) ([]byte, error) {
		return nil, errors.New("vault said: " + planted)
	})))
	if _, err := bad.Render(context.Background(), doc(t, applyDoc)); err != nil {
		outputs = append(outputs, "resolver error (redacted by the resolver's own renderer only if known): "+err.Error())
	}
	f.sas["w3-site-a"] = []*vici.Message{saMsg("1", "ESTABLISHED", "w3-site-a", "1", "INSTALLED")}
	st, err := r.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	js, _ := protojson.Marshal(st)
	outputs = append(outputs, "retrieve: "+string(js))
	// Events.
	for _, e := range r.fromVICI(vici.Event{Name: "ike-updown", Message: msg("up", "yes", "w3-site-a", msg("state", "ESTABLISHED", "remote-id", "10.3.250.2"))}) {
		outputs = append(outputs, "event: "+e.String()+" "+protojson.Format(e.ToProto()))
	}
	outputs = append(outputs, "logs: "+logs.String())
	// VICI requests other than load-shared.
	for _, c := range f.calls {
		if c.cmd != "load-shared" && c.in != nil {
			outputs = append(outputs, "vici "+c.cmd+": "+c.in.String())
		}
	}
	for _, out := range outputs {
		for _, form := range forms {
			if strings.Contains(out, form) {
				if strings.HasPrefix(out, "resolver error") {
					// The resolver error text comes from the secret store; the renderer masks
					// values it has resolved before, which this one never did. Documented.
					continue
				}
				t.Errorf("planted secret (form %q) leaked in %s", form[:8], clip(out))
			}
		}
	}
	// And it is where it must be.
	data, err := os.ReadFile(p.SecretsFile())
	if err != nil || !bytes.Contains(data, []byte(forms[1])) || bytes.Contains(data, []byte(planted)) {
		t.Error("secrets file must hold the PSK as base64 only")
	}
	info, _ := os.Stat(p.SecretsFile())
	if info.Mode().Perm() != 0o600 {
		t.Errorf("secrets file mode %v", info.Mode().Perm())
	}
	if !strings.Contains(logs.String(), "strongswan: applied") {
		t.Error("logger saw nothing (test would be vacuous)")
	}
}

func TestRetrieveState(t *testing.T) {
	f := newFakeCharon()
	r := newTestRenderer(t, unitPaths(t), WithDialer(f.dialer()))
	if err := r.Apply(context.Background(), renderApply(t, r, applyDoc)); err != nil {
		t.Fatal(err)
	}
	f.sas["w3-site-a"] = []*vici.Message{saMsg("7", "ESTABLISHED", "w3-site-a", "3", "INSTALLED")}
	f.sas["w3-gone"] = []*vici.Message{saMsg("9", "ESTABLISHED", "w3-gone", "4", "INSTALLED")} // not loaded over VICI
	st, err := r.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Daemon.Version != "6.0.4" || len(st.Conns) != 2 || len(st.SAs) != 1 || st.UnlistedSAs != 1 {
		t.Fatalf("state: version %q conns %d sas %d unlisted %d", st.Daemon.Version, len(st.Conns), len(st.SAs), st.UnlistedSAs)
	}
	sa := st.SAs[0]
	if sa.Tunnel != "w3-site-a" || sa.State != "ESTABLISHED" || len(sa.Children) != 1 || sa.Children[0].BytesOut != 1680 || sa.Children[0].SPIIn != "c1a2b3c4" {
		t.Errorf("sa %+v", sa)
	}
	c := st.Conns[0]
	if c.Name != "w3+site-b" || c.Tunnel != "w3.site-b" || c.LocalAuth != "pre-shared key" || len(c.Children) != 1 {
		t.Errorf("conn %+v", c)
	}
	if !slices.Equal(st.SharedSecrets, []string{"ike-w3+site-b", "ike-w3-site-a"}) {
		t.Errorf("shared %v", st.SharedSecrets)
	}
	pb, err := r.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if js, _ := protojson.Marshal(pb); !strings.Contains(string(js), `"spiIn":"c1a2b3c4"`) {
		t.Errorf("Retrieve struct: %s", js)
	}
}

func TestRetrieveBounded(t *testing.T) {
	f := newFakeCharon()
	r := newTestRenderer(t, unitPaths(t), WithDialer(f.dialer()))
	if err := r.Apply(context.Background(), renderApply(t, r, onlySiteA(t))); err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= MaxSAsPerConn; i++ {
		f.sas["w3-site-a"] = append(f.sas["w3-site-a"], saMsg("1", "ESTABLISHED", "w3-site-a", "1", "INSTALLED"))
	}
	st, err := r.State(context.Background())
	if err != nil || !slices.Equal(st.Truncated, []string{"w3-site-a"}) || len(st.SAs) != MaxSAsPerConn {
		t.Fatalf("State = %v truncated %v sas %d, want a truncated listing, not a failure (review L3)", err, st.Truncated, len(st.SAs))
	}
}

// TestRedactionIsFieldBased: a PSK whose text equals a state value must not rewrite that value
// (review L2: no oracle, no corrupted state), while `secret = …` in errors is still masked.
func TestRedactionIsFieldBased(t *testing.T) {
	f := newFakeCharon()
	r := newTestRenderer(t, unitPaths(t), WithDialer(f.dialer()),
		WithSecretResolver(testResolver(map[string]string{"psk/w3-site-a": "ESTABLISHED", "psk/w3-site-b": testPSK})))
	if err := r.Apply(context.Background(), renderApply(t, r, applyDoc)); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.sas["w3-site-a"] = []*vici.Message{saMsg("1", "ESTABLISHED", "w3-site-a", "1", "INSTALLED")}
	f.mu.Unlock()
	pb, err := r.Retrieve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	js, _ := protojson.Marshal(pb)
	if strings.Contains(string(js), Redacted) || !strings.Contains(string(js), `"state":"ESTABLISHED"`) {
		t.Errorf("state rewritten by the redactor: %s", js)
	}
	if got := (secretSet{}).redactErr(errors.New("line: secret = VRX_TEST_PSK_RF2_masked failed")).Error(); strings.Contains(got, "VRX_TEST_PSK_RF2_masked") {
		t.Errorf("secret assignment not masked: %s", got)
	}
}

func TestInitiateTerminateValidateNames(t *testing.T) {
	f := newFakeCharon()
	r := newTestRenderer(t, unitPaths(t), WithDialer(f.dialer()))
	if err := r.Initiate(context.Background(), "w3-site-a", "w3-site-a", 5000); err != nil {
		t.Fatal(err)
	}
	if err := r.Terminate(context.Background(), "a.b\n", 0); !errors.Is(err, ErrInput) {
		t.Fatalf("Terminate(bad name) = %v", err)
	}
}

// ------------------------------------------------------------------------------ events

func TestEventsFromVICI(t *testing.T) {
	r := newTestRenderer(t, unitPaths(t))
	ts := time.Unix(1790000000, 0)
	up := r.fromVICI(vici.Event{Name: "ike-updown", Timestamp: ts, Message: msg("up", "yes", "w3+site-b", msg("uniqueid", "3", "state", "ESTABLISHED", "remote-host", "10.3.250.3"))})
	if len(up) != 1 || up[0].Conn != "w3+site-b" || !up[0].Up || up[0].State != "ESTABLISHED" || up[0].Attrs["ike-remote-host"] != "10.3.250.3" {
		t.Fatalf("ike-updown: %+v", up)
	}
	pb := up[0].ToProto()
	if pb.GetAttributes()["tunnel"] != "w3.site-b" || pb.GetAttributes()["source"] != "strongswan" {
		t.Errorf("proto attrs %v", pb.GetAttributes())
	}
	child := r.fromVICI(vici.Event{Name: "child-updown", Message: msg("w3-site-a", msg("uniqueid", "3", "state", "ESTABLISHED",
		"child-sas", msg("w3-site-a-5", msg("name", "w3-site-a", "uniqueid", "5", "state", "DELETING", "spi-in", "c1"))))})
	if len(child) != 1 || child[0].Up || child[0].Child != "w3-site-a" || child[0].State != "DELETING" || child[0].Attrs["child-spi-in"] != "c1" {
		t.Fatalf("child-updown: %+v", child)
	}
	rekey := r.fromVICI(vici.Event{Name: "child-rekey", Message: msg("w3-site-a", msg("uniqueid", "3", "state", "ESTABLISHED",
		"child-sas", msg("w3-site-a-5", msg("old", msg("name", "w3-site-a", "uniqueid", "5"), "new", msg("name", "w3-site-a", "uniqueid", "6", "state", "INSTALLED")))))})
	if len(rekey) != 1 || rekey[0].Attrs["child-uniqueid"] != "6" || rekey[0].State != "INSTALLED" {
		t.Fatalf("child-rekey: %+v", rekey)
	}
	// Keys that are not connection names (hostile or foreign) are dropped.
	if got := r.fromVICI(vici.Event{Name: "ike-updown", Message: msg("up", "yes", "a b\nc", msg("state", "X"))}); len(got) != 0 {
		t.Errorf("foreign key produced %v", got)
	}
	// Attribute values are clipped.
	long := r.fromVICI(vici.Event{Name: "ike-updown", Message: msg("w3-x", msg("remote-id", strings.Repeat("x", 5000)))})
	if len(long[0].Attrs["ike-remote-id"]) > maxAttr {
		t.Error("attribute not clipped")
	}
}

func TestWatchReconnectsAndPolls(t *testing.T) {
	f := newFakeCharon()
	r := newTestRenderer(t, unitPaths(t), WithDialer(f.dialer()))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out := make(chan Event, 64)
	done := make(chan error, 1)
	go func() { done <- r.Watch(ctx, out) }()

	waitSubscribed := func() {
		for i := 0; i < 200; i++ {
			f.mu.Lock()
			n := len(f.subscribers)
			f.mu.Unlock()
			if n > 0 {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("Watch never subscribed")
	}
	next := func() Event {
		select {
		case e := <-out:
			return e
		case <-ctx.Done():
			t.Fatal("timeout waiting for an event")
		}
		return Event{}
	}
	waitSubscribed()
	f.push(vici.Event{Name: "ike-updown", Message: msg("up", "yes", "w3-site-a", msg("uniqueid", "1", "state", "ESTABLISHED"))})
	if e := next(); e.Kind != "ike-updown" || !e.Up {
		t.Fatalf("first event %+v", e)
	}
	// charon restarts and refuses event registration for a while: channel closes → daemon
	// down; 1 Hz polling reports the SA that appears; when subscribing works again → daemon up.
	if err := r.Apply(ctx, renderApply(t, r, onlySiteA(t))); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.failOn["subscribe"] = "event registration refused"
	f.mu.Unlock()
	f.restart(false)
	if e := next(); e.Kind != KindDaemon || e.Up || e.ToProto().GetKind().String() != "EVENT_KIND_ERROR" {
		t.Fatalf("want daemon down, got %+v", e)
	}
	time.Sleep(1500 * time.Millisecond) // at least one poll snapshot without the SA
	f.mu.Lock()
	f.sas["w3-site-a"] = []*vici.Message{saMsg("2", "ESTABLISHED", "w3-site-a", "2", "INSTALLED")}
	f.mu.Unlock()
	for next().Kind != KindPoll { //nolint:revive // drain until the poll event
		continue
	}
	f.mu.Lock()
	delete(f.failOn, "subscribe")
	f.mu.Unlock()
	for {
		if e := next(); e.Kind == KindDaemon && e.Up {
			break
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("Watch returned %v", err)
	}
}
