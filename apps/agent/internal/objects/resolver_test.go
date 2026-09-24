package objects

import (
	"bytes"
	"context"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// fakeClock is the resolver's clock in tests; the resolver loop is not started, the test calls
// ResolveDue after moving the clock.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// logBuf collects the resolver's log lines (evidence: they are printed with t.Log).
type logBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuf) Write(p []byte) (int, error) { l.mu.Lock(); defer l.mu.Unlock(); return l.b.Write(p) }
func (l *logBuf) String() string              { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func openRT(t *testing.T, dir string, dns *dnsResponder, clock *fakeClock, lb *logBuf) *Runtime {
	t.Helper()
	rt, err := Open(Config{
		StateDir: dir, Owner: "w3", Lookup: NetLookup([]string{dns.addr}), Now: clock.Now,
		Log: slog.New(slog.NewTextHandler(lb, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	return rt
}

func fqdnObject(host string) *vrxv1.AddressObject {
	return &vrxv1.AddressObject{Type: ptr("fqdn"), Fqdn: ptr(host)}
}

func addrsOf(rt *Runtime, name string) string {
	as, _ := rt.FQDN(name)
	return strings.Join(addrStrings(as), " ")
}

// The acceptance run: an FQDN object for a name served by the in-process responder resolves,
// the fixed-interval refresh picks up a changed answer, a dead resolver keeps the last-good
// answer, and a restarted runtime reloads the state without re-querying.
func TestFQDNResolveRefreshLastGoodRestart(t *testing.T) {
	dns := startDNS(t)
	dns.set("web.w3.test", "192.0.2.10", "2001:db8::10")
	dns.set("v4only.w3.test", "198.51.100.1")
	clock := &fakeClock{t: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	dir := t.TempDir()
	lb := &logBuf{}
	rt := openRT(t, dir, dns, clock, lb)
	var changes []Change
	var cmu sync.Mutex
	unsub := rt.Subscribe(func(c Change) { cmu.Lock(); changes = append(changes, c); cmu.Unlock() })
	defer unsub()

	// two objects share one name (resolved once), a third is v4-only (AAAA = NODATA, not an error)
	for name, host := range map[string]string{"web": "web.w3.test", "web-alias": "WEB.w3.test.", "v4": "v4only.w3.test"} {
		if err := rt.Store().put(KindAddresses, name, fqdnObject(host)); err != nil {
			t.Fatal(err)
		}
	}
	if n := rt.ResolveDue(context.Background()); n != 2 {
		t.Fatalf("resolved %d names, want 2 (web and v4only)", n)
	}
	if addrsOf(rt, "web") != "192.0.2.10 2001:db8::10" || addrsOf(rt, "web-alias") != "192.0.2.10 2001:db8::10" || addrsOf(rt, "v4") != "198.51.100.1" {
		t.Fatalf("resolved: web=%q alias=%q v4=%q", addrsOf(rt, "web"), addrsOf(rt, "web-alias"), addrsOf(rt, "v4"))
	}
	st := rt.FQDNStates("web")
	if len(st) != 1 || st[0].Err != "" || !st[0].LastResolved.Equal(clock.Now()) || !st[0].NextRefresh.Equal(clock.Now().Add(DefaultRefresh)) {
		t.Fatalf("state after the first resolution: %+v", st)
	}
	if v := rt.FQDNStates("v4"); v[0].Err != "" {
		t.Fatalf("NODATA for AAAA is an answer, not an error: %+v", v)
	}
	// Expand sees the resolver's answer through WithFQDN
	if got, err := Expand(rt.Snapshot(), "web", WithFQDN(rt.FQDN)); err != nil || prefixes(got.All()) != "192.0.2.10/32 2001:db8::10/128" {
		t.Fatalf("Expand: %v %v", got, err)
	}

	// refresh (fixed interval): nothing is queried before it is due; the new answer is picked up at +60 s
	dns.set("web.w3.test", "192.0.2.11", "2001:db8::10")
	before := dns.count("web.w3.test")
	clock.Advance(DefaultRefresh - time.Second)
	if n := rt.ResolveDue(context.Background()); n != 0 || dns.count("web.w3.test") != before {
		t.Fatalf("queried before the refresh was due: %d", n)
	}
	clock.Advance(time.Second)
	rt.ResolveDue(context.Background())
	if addrsOf(rt, "web") != "192.0.2.11 2001:db8::10" {
		t.Fatalf("refresh not observed: %q", addrsOf(rt, "web"))
	}
	cmu.Lock()
	if len(changes) != 3 || changes[2].Host != "web.w3.test" || strings.Join(changes[2].Objects, ",") != "web,web-alias" {
		t.Fatalf("change notifications: %+v", changes)
	}
	cmu.Unlock()

	// resolver down: last-good kept, error recorded, retry after 30 s, then 60 s (≤ the interval)
	dns.stop()
	clock.Advance(DefaultRefresh)
	rt.ResolveDue(context.Background())
	st = rt.FQDNStates("web")
	if addrsOf(rt, "web") != "192.0.2.11 2001:db8::10" || st[0].Err == "" || st[0].Failures != 1 || !st[0].NextRefresh.Equal(clock.Now().Add(MinRefresh)) {
		t.Fatalf("down: addresses %q state %+v", addrsOf(rt, "web"), st)
	}
	clock.Advance(MinRefresh)
	rt.ResolveDue(context.Background())
	if st = rt.FQDNStates("web"); st[0].Failures != 2 || !st[0].NextRefresh.Equal(clock.Now().Add(DefaultRefresh)) {
		t.Fatalf("second failure: %+v", st)
	}
	t.Logf("resolver log:\n%s", lb.String())
	if !strings.Contains(lb.String(), "fqdn resolution failed; last-good addresses kept") {
		t.Fatal("no last-good log line")
	}

	// restart: a new runtime on the same state dir has the answers at once and queries nothing
	// while they are fresh (a responder on a new port would see every query)
	rt.Close()
	dns2 := startDNS(t)
	dns2.set("web.w3.test", "192.0.2.12")
	dns2.set("v4only.w3.test", "198.51.100.1")
	clock.Advance(5 * time.Second)
	lb2 := &logBuf{}
	rt2 := openRT(t, dir, dns2, clock, lb2)
	if addrsOf(rt2, "web") != "192.0.2.11 2001:db8::10" || addrsOf(rt2, "v4") != "198.51.100.1" {
		t.Fatalf("not reloaded: web=%q v4=%q", addrsOf(rt2, "web"), addrsOf(rt2, "v4"))
	}
	if n := rt2.ResolveDue(context.Background()); n != 0 || rt2.Queries() != 0 || dns2.count("web.w3.test")+dns2.count("v4only.w3.test") != 0 {
		t.Fatalf("restart re-queried: resolved %d, queries %d", n, rt2.Queries())
	}
	t.Logf("restart log:\n%s", lb2.String())
	if !strings.Contains(lb2.String(), `msg="fqdn state reloaded"`) || !strings.Contains(lb2.String(), "fresh=2 due=0") {
		t.Fatalf("reload log: %s", lb2.String())
	}
}

// After a longer outage every name is overdue: a restart spreads them over RestartWindow instead
// of querying all of them at once.
func TestFQDNRestartSpreadsOverdue(t *testing.T) {
	dns := startDNS(t)
	clock := &fakeClock{t: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	dir := t.TempDir()
	rt := openRT(t, dir, dns, clock, &logBuf{})
	for i := 0; i < 10; i++ {
		host := string(rune('a'+i)) + ".w3.test"
		dns.set(host, netip.AddrFrom4([4]byte{192, 0, 2, byte(i + 1)}).String())
		if err := rt.Store().put(KindAddresses, "o"+host[:1], fqdnObject(host)); err != nil {
			t.Fatal(err)
		}
	}
	if n := rt.ResolveDue(context.Background()); n != 10 {
		t.Fatalf("first round: %d", n)
	}
	rt.Close()

	clock.Advance(2 * time.Hour) // the agent was down for two hours
	rt2 := openRT(t, dir, dns, clock, &logBuf{})
	now := clock.Now()
	seen := map[time.Time]bool{}
	for _, s := range rt2.FQDNStates() {
		if s.NextRefresh.Before(now) || !s.NextRefresh.Before(now.Add(RestartWindow)) || seen[s.NextRefresh] || len(s.Addresses) != 1 {
			t.Fatalf("not spread over %v: %+v", RestartWindow, s)
		}
		seen[s.NextRefresh] = true
	}
	if n := rt2.ResolveDue(context.Background()); n != 1 {
		t.Fatalf("right after the restart %d names were queried, want 1", n)
	}
	clock.Advance(RestartWindow)
	if n := rt2.ResolveDue(context.Background()); n != 9 {
		t.Fatalf("after the window: %d", n)
	}
}

// Removing the object drops its name from the resolver and from the state file; a name that
// never resolved expands to nothing and says so.
func TestFQDNSyncAndUnresolved(t *testing.T) {
	dns := startDNS(t)
	clock := &fakeClock{t: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	dir := t.TempDir()
	rt := openRT(t, dir, dns, clock, &logBuf{})
	if err := rt.Store().put(KindAddresses, "nx", fqdnObject("nx.w3.test")); err != nil {
		t.Fatal(err)
	}
	rt.ResolveDue(context.Background())
	st := rt.FQDNStates()
	if len(st) != 1 || len(st[0].Addresses) != 0 || !st[0].LastResolved.IsZero() || st[0].Err == "" {
		t.Fatalf("NXDOMAIN: %+v", st)
	}
	got, err := Expand(rt.Snapshot(), "nx", WithFQDN(rt.FQDN))
	if err != nil || got.Len() != 0 || strings.Join(got.Unresolved, ",") != "nx" {
		t.Fatalf("unresolved expansion: %+v %v", got, err)
	}
	if err := rt.Store().remove(KindAddresses, "nx"); err != nil {
		t.Fatal(err)
	}
	if len(rt.FQDNStates()) != 0 {
		t.Fatal("state kept for a removed object")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "objects-fqdn-w3.json")) //nolint:gosec // the test's own temp dir
	if err != nil || strings.Contains(string(raw), "nx.w3.test") {
		t.Fatalf("state file still has the name: %v\n%s", err, raw)
	}
}

func TestClampRefreshAndRetry(t *testing.T) {
	for in, want := range map[time.Duration]time.Duration{0: DefaultRefresh, time.Second: MinRefresh, 5 * time.Minute: 5 * time.Minute, 3 * time.Hour: MaxRefresh} {
		if got := ClampRefresh(in); got != want {
			t.Errorf("ClampRefresh(%v) = %v, want %v", in, got, want)
		}
	}
	r := newResolver("", nil, 10*time.Minute, time.Now, slog.Default())
	var got []string
	for n := 1; n <= 7; n++ {
		got = append(got, r.retryDelay(n).String())
	}
	if strings.Join(got, " ") != "30s 1m0s 2m0s 4m0s 8m0s 10m0s 10m0s" {
		t.Fatalf("retry delays %v", got)
	}
	// a TTL-reporting lookup is honoured within the same bounds
	if d := r.interval(familyResult{ttl: 5 * time.Second}, familyResult{ttl: 300 * time.Second}); d != MinRefresh {
		t.Fatalf("ttl 5s → %v", d)
	}
	if d := r.interval(familyResult{ttl: 300 * time.Second}); d != 5*time.Minute {
		t.Fatalf("ttl 300s → %v", d)
	}
}

func slogTo(lb *logBuf) *slog.Logger {
	return slog.New(slog.NewTextHandler(lb, &slog.HandlerOptions{Level: slog.LevelDebug}))
}
