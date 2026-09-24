package strongswan

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/strongswan/govici/vici"
	"google.golang.org/protobuf/encoding/protojson"
)

// variant returns applyDoc (site-a only) with replacements applied to the JSON text.
func variant(t *testing.T, repl ...string) string {
	t.Helper()
	s := onlySiteA(t)
	for i := 0; i+1 < len(repl); i += 2 {
		if !strings.Contains(s, repl[i]) {
			t.Fatalf("fixture: %q not in %s", repl[i], s)
		}
		s = strings.Replace(s, repl[i], repl[i+1], 1)
	}
	return s
}

func establish(t *testing.T, r *Renderer, f *fakeCharon) string {
	t.Helper()
	if err := r.Initiate(context.Background(), "w3-site-a", "w3-site-a", 1000); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := "5" + strings.Repeat("0", f.nextID)
	f.sas["w3-site-a"] = []*vici.Message{saMsg(id, "ESTABLISHED", "w3-site-a", id, "INSTALLED")}
	sa := f.sas["w3-site-a"][0]
	_ = sa.Set("remote-id", "10.3.250.2")
	return id
}

func liveSAs(f *fakeCharon, conn string) []*vici.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.sas[conn])
}

// H1: a narrowed selector terminates the CHILD_SA; a rotated PSK terminates the IKE_SA.
func TestApplyReestablishesChangedSAs(t *testing.T) {
	f := newFakeCharon()
	secrets := map[string]string{"psk/w3-site-a": testPSK + "_one"}
	r := newTestRenderer(t, unitPaths(t), WithDialer(f.dialer()), WithSecretResolver(testResolver(secrets)))
	if err := r.Apply(context.Background(), renderApply(t, r, onlySiteA(t))); err != nil {
		t.Fatal(err)
	}
	id := establish(t, r, f)

	// Narrow the remote selector: the CHILD_SA (10.3.2.0/24) must go, the IKE_SA stays.
	f.terminated = nil
	if err := r.Apply(context.Background(), renderApply(t, r, variant(t, `"10.3.2.0/24"`, `"10.3.2.0/25"`))); err != nil {
		t.Fatalf("Apply narrowed: %v", err)
	}
	if len(f.terminated) != 1 || !strings.Contains(f.terminated[0], "child-id = "+id) {
		t.Fatalf("terminated %v, want the CHILD_SA #%s", f.terminated, id)
	}
	if sas := liveSAs(f, "w3-site-a"); len(sas) != 1 || len(childSections(sas[0])) != 0 {
		t.Fatalf("live SAs after narrowing: %v", sas)
	}

	// Rotate the PSK: the IKE_SA negotiated with the old key must go.
	establish(t, r, f)
	sid := str(liveSAs(f, "w3-site-a")[0], "uniqueid")
	f.terminated = nil
	secrets["psk/w3-site-a"] = testPSK + "_two"
	if err := r.Apply(context.Background(), renderApply(t, r, variant(t, `"10.3.2.0/24"`, `"10.3.2.0/25"`))); err != nil {
		t.Fatalf("Apply rotated: %v", err)
	}
	if len(f.terminated) != 1 || !strings.Contains(f.terminated[0], "ike-id = "+sid) {
		t.Fatalf("terminated %v, want IKE_SA #%s", f.terminated, sid)
	}
	st, err := r.State(context.Background())
	if err != nil || st.StaleSAs != 0 {
		t.Fatalf("state %v stale %d", err, st.StaleSAs)
	}
}

// H1 without a previous plan: a live SA that visibly contradicts the config is terminated.
func TestApplyTerminatesContradictingSAWithoutHistory(t *testing.T) {
	f := newFakeCharon()
	r := newTestRenderer(t, unitPaths(t), WithDialer(f.dialer()))
	wide := saMsg("7", "ESTABLISHED", "w3-site-a", "7", "INSTALLED")
	cs := sub(wide, "child-sas")
	_ = sub(cs, cs.Keys()[0]).Set("remote-ts", []string{"10.3.0.0/16"})
	_ = wide.Set("remote-id", "10.3.250.2")
	f.sas["w3-site-a"] = []*vici.Message{wide}
	if err := r.Apply(context.Background(), renderApply(t, r, onlySiteA(t))); err != nil {
		t.Fatal(err)
	}
	if len(f.terminated) != 1 || !strings.Contains(f.terminated[0], "child-id = 7") {
		t.Fatalf("terminated %v", f.terminated)
	}
}

// H1: a terminate charon cannot carry out fails the Apply (and rolls back).
func TestApplyFailsWhenStaleSAStays(t *testing.T) {
	f := newFakeCharon()
	r := newTestRenderer(t, unitPaths(t), WithDialer(f.dialer()))
	if err := r.Apply(context.Background(), renderApply(t, r, onlySiteA(t))); err != nil {
		t.Fatal(err)
	}
	establish(t, r, f)
	f.failOn["terminate"] = "cannot"
	err := r.Apply(context.Background(), renderApply(t, r, variant(t, `"10.3.2.0/24"`, `"10.3.2.0/25"`)))
	if err == nil {
		t.Fatal("Apply succeeded while the old CHILD_SA stayed up")
	}
}

// M1: soft edits keep the SA; none→start does not duplicate an existing CHILD_SA; a start child
// without a CHILD_SA is initiated exactly once.
func TestApplySoftChangesKeepSAs(t *testing.T) {
	f := newFakeCharon()
	r := newTestRenderer(t, unitPaths(t), WithDialer(f.dialer()))
	if err := r.Apply(context.Background(), renderApply(t, r, onlySiteA(t))); err != nil {
		t.Fatal(err)
	}
	id := establish(t, r, f)
	f.terminated, f.initiated = nil, nil
	soft := variant(t, `"startAction":"none"`, `"startAction":"start","dpd":{"enabled":true,"delaySec":20},"rekey":{"ikeSec":7200}`)
	files := renderApply(t, r, soft)
	impact, err := r.Impact(files)
	if err != nil || impact["w3-site-a"] != "update" {
		t.Fatalf("Impact = %v %v, want update", impact, err)
	}
	if err := r.Apply(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	if len(f.terminated) != 0 || len(f.initiated) != 0 {
		t.Fatalf("soft change disturbed the SA: terminated %v initiated %v", f.terminated, f.initiated)
	}
	if sas := liveSAs(f, "w3-site-a"); len(sas) != 1 || str(sas[0], "uniqueid") != id {
		t.Fatalf("SA replaced: %v", sas)
	}
	// start_action start is never handed to charon (it would undo/redo it on every replace).
	if c := sub(sub(f.conns["w3-site-a"], "children"), "w3-site-a"); str(c, "start_action") != "" {
		t.Fatalf("loaded start_action %q", str(c, "start_action"))
	}
	// Without a CHILD_SA the renderer initiates it — once.
	f.mu.Lock()
	delete(f.sas, "w3-site-a")
	f.mu.Unlock()
	for i := 0; i < 2; i++ {
		if err := r.Apply(context.Background(), files); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.initiated) != 1 {
		t.Fatalf("initiated %v, want exactly one", f.initiated)
	}
	if im, _ := r.Impact(renderApply(t, r, variant(t, `"10.3.2.0/24"`, `"10.3.2.0/25"`))); im["w3-site-a"] != "reestablish" {
		t.Fatalf("Impact(narrowed) = %v", im)
	}
}

// M2: no implicit product paths; the owner prefix protects foreign objects.
func TestOwnerPrefix(t *testing.T) {
	f := newFakeCharon()
	f.conns["w4-other"] = msg("version", "2")
	f.connOrder = append(f.connOrder, "w4-other")
	f.shared["ike-w4-other"] = sharedSecret{id: "ike-w4-other"}
	r := newTestRenderer(t, unitPaths(t), WithDialer(f.dialer()), WithOwnerPrefix("w3"))
	if err := r.Apply(context.Background(), renderApply(t, r, applyDoc)); err != nil {
		t.Fatal(err)
	}
	if got := f.loadedConns(); !slices.Contains(got, "w4-other") {
		t.Fatalf("foreign connection unloaded: %v", got)
	}
	if got := f.loadedShared(); !slices.Contains(got, "ike-w4-other") {
		t.Fatalf("foreign secret unloaded: %v", got)
	}
	st, err := r.State(context.Background())
	if err != nil || len(st.Conns) != 2 || slices.Contains(st.SharedSecrets, "ike-w4-other") {
		t.Fatalf("State leaks foreign objects: %v %+v", err, st)
	}
	// Removing everything we own leaves the foreign ones.
	empty, err := r.Render(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(context.Background(), empty); err != nil {
		t.Fatal(err)
	}
	if got := f.loadedConns(); !slices.Equal(got, []string{"w4-other"}) {
		t.Fatalf("after empty apply: %v", got)
	}
	// A name without the prefix is refused.
	if _, err := r.Render(context.Background(), tunnelWith(t, "site-x", func(map[string]any) {})); !errors.Is(err, ErrInput) {
		t.Fatalf("unprefixed tunnel rendered: %v", err)
	}
}

// M3: a changed charon start time is reported until acknowledged; Watch reports it on subscribe.
func TestRestartDetection(t *testing.T) {
	f := newFakeCharon()
	r := newTestRenderer(t, unitPaths(t), WithDialer(f.dialer()))
	st, err := r.State(context.Background())
	if err != nil || st.Restarted || st.DaemonStartedAt == "" {
		t.Fatalf("first observation: %v %+v", err, st)
	}
	f.mu.Lock()
	f.since = "Sep 24 03:00:00 2026"
	f.mu.Unlock()
	for i := 0; i < 2; i++ { // persists until acknowledged
		if st, err = r.State(context.Background()); err != nil || !st.Restarted || st.AckedStartedAt != "Sep 24 01:00:00 2026" {
			t.Fatalf("after restart: %v %+v", err, st)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan Event, 8)
	go func() { _ = r.Watch(ctx, out) }()
	e := <-out
	cancel()
	if e.Kind != KindDaemon || e.State != "restarted" || e.Attrs["restarted"] != "yes" {
		t.Fatalf("watch event %+v", e)
	}
	if err := r.AckRestart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st, err = r.State(context.Background()); err != nil || st.Restarted {
		t.Fatalf("after ack: %v %+v", err, st)
	}
	pb, _ := r.Retrieve(context.Background())
	if js, _ := protojson.Marshal(pb); !strings.Contains(string(js), `"restarted":false`) {
		t.Fatalf("Retrieve: %s", js)
	}
}

// M4: a full event buffer (govici would drop) triggers a resync, and the periodic resync
// repairs a change no event reported.
func TestWatchResync(t *testing.T) {
	f := newFakeCharon()
	r := newTestRenderer(t, unitPaths(t), WithDialer(f.dialer()), WithResyncInterval(300*time.Millisecond))
	if err := r.Apply(context.Background(), renderApply(t, r, onlySiteA(t))); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out := make(chan Event) // unbuffered: a stalled consumer
	go func() { _ = r.Watch(ctx, out) }()
	for i := 0; i < 200; i++ {
		f.mu.Lock()
		n := len(f.subscribers)
		f.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Stalled consumer: flood more events than the buffer holds.
	for i := 0; i < 400; i++ {
		f.push(vici.Event{Name: "ike-updown", Message: msg("up", "yes", "w3-site-a", msg("uniqueid", "1", "state", "ESTABLISHED"))})
	}
	// An SA appears without any event.
	f.mu.Lock()
	f.sas["w3-site-a"] = []*vici.Message{saMsg("9", "ESTABLISHED", "w3-site-a", "9", "INSTALLED")}
	f.mu.Unlock()
	var resync, poll bool
	for !resync || !poll {
		select {
		case e := <-out:
			resync = resync || e.Kind == KindResync
			poll = poll || (e.Kind == KindPoll && e.Conn == "w3-site-a" && e.Up)
		case <-ctx.Done():
			t.Fatalf("resync %v poll %v", resync, poll)
		}
	}
}

// L1: every derived name fits 64 characters; longer tunnel names are refused before render.
func TestTunnelNameCap(t *testing.T) {
	ok := "w3-" + strings.Repeat("a", MaxConnNameLen-3)
	if _, err := ConnName(ok); err != nil {
		t.Fatalf("%d-char name: %v", len(ok), err)
	}
	r := newTestRenderer(t, goldenPaths(), WithSecretResolver(testResolver(map[string]string{"psk/w3-site-a": testPSK})))
	if _, err := r.Render(context.Background(), tunnelWith(t, ok, func(map[string]any) {})); err != nil {
		t.Fatalf("render %d-char name with PSK: %v", len(ok), err)
	}
	long := ok + "b"
	_, err := r.Render(context.Background(), tunnelWith(t, long, func(map[string]any) {}))
	if !errors.Is(err, ErrInput) || !strings.Contains(err.Error(), "limited to 60") {
		t.Fatalf("61-char name: %v", err)
	}
}
