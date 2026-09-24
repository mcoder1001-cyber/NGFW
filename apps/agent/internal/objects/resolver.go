package objects

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// Refresh policy of the FQDN resolver.
const (
	// MinRefresh and MaxRefresh bound every refresh interval (a configured one and a TTL alike).
	MinRefresh = 30 * time.Second
	MaxRefresh = time.Hour
	// DefaultRefresh is the fixed interval used when the lookup reports no TTL (Go's resolver never
	// does); VRX_OBJECTS_FQDN_REFRESH_SEC overrides it (subsystems/object_model.go).
	DefaultRefresh = 60 * time.Second
	// LookupTimeout bounds one resolution (A and AAAA in parallel, every server).
	LookupTimeout = 5 * time.Second
	// RestartWindow: after a (re)start, entries that are due (overdue while the agent was down,
	// or never resolved) are spread evenly over this window instead of being queried at once.
	RestartWindow = 30 * time.Second
	// DormantTTL: how long the answers of a name no object uses any more are kept (not queried).
	DormantTTL = time.Hour
	// queryBurst lookups may run back to back; after that the loop waits querySpacing between
	// rounds (≤ 16 hosts per second), so a bulk commit of new FQDN objects is no query storm either.
	queryBurst   = 4
	querySpacing = 250 * time.Millisecond
)

// ClampRefresh bounds d to [MinRefresh, MaxRefresh]; 0 means DefaultRefresh.
func ClampRefresh(d time.Duration) time.Duration {
	switch {
	case d == 0:
		return DefaultRefresh
	case d < MinRefresh:
		return MinRefresh
	case d > MaxRefresh:
		return MaxRefresh
	}
	return d
}

// Lookup resolves one address family of a host name.
type Lookup interface {
	// LookupFamily returns the addresses of host (fully qualified, with the trailing dot) for
	// network "ip4" (A) or "ip6" (AAAA) and their TTL (0 = unknown: the fixed refresh interval
	// applies). A family without records fails with an error for which IsNotFound is true.
	LookupFamily(ctx context.Context, network, host string) ([]netip.Addr, time.Duration, error)
}

// IsNotFound reports whether err says the name (or the family) has no records — an answer, not a
// resolver failure.
func IsNotFound(err error) bool {
	var de *net.DNSError
	return errors.As(err, &de) && de.IsNotFound
}

// NetLookup resolves with Go's own resolver (PreferGo: no cgo, no exec, no shell). With no
// servers it is the system resolver configuration (/etc/resolv.conf, and /etc/hosts per
// nsswitch.conf); otherwise each query goes to these "host:port" servers, in order (tests: the
// in-process responder; VRX_OBJECTS_DNS_SERVERS). It reports no TTL.
func NetLookup(servers []string) Lookup {
	if len(servers) == 0 {
		return netLookup{resolvers: []*net.Resolver{{PreferGo: true}}}
	}
	l := netLookup{servers: append([]string(nil), servers...)}
	for _, s := range servers {
		server := s
		l.resolvers = append(l.resolvers, &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, server)
		}})
	}
	return l
}

type netLookup struct {
	resolvers []*net.Resolver
	servers   []string // the override servers, index-aligned with resolvers (nil: system configuration)
}

func (l netLookup) LookupFamily(ctx context.Context, network, host string) ([]netip.Addr, time.Duration, error) {
	var err error
	for i, r := range l.resolvers {
		var addrs []netip.Addr
		addrs, err = r.LookupNetIP(ctx, network, host)
		var de *net.DNSError
		if i < len(l.servers) && errors.As(err, &de) {
			// Go names the resolv.conf server it would have used; name the one actually asked
			cp := *de
			cp.Server = l.servers[i]
			err = &cp
		}
		if err == nil {
			out := make([]netip.Addr, 0, len(addrs))
			for _, a := range addrs {
				a = a.Unmap()
				if (network == "ip4") == a.Is4() {
					out = append(out, a)
				}
			}
			return out, 0, nil
		}
		if IsNotFound(err) || ctx.Err() != nil {
			break
		}
	}
	return nil, 0, err
}

// canonHost is the resolver's key of an FQDN: lower case, no trailing dot.
func canonHost(s string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
}

// Change is one change of the addresses an FQDN resolves to (Runtime.Subscribe).
type Change struct {
	// Host is the resolved name; Objects the FQDN address objects that use it (sorted).
	Host    string
	Objects []string
	// Addresses now in use (IPv4 first, each family sorted); empty = the objects expand to nothing.
	Addresses []netip.Addr
}

// FQDNState is the resolver's view of one FQDN address object (FqdnObjectState RPC).
type FQDNState struct {
	Name         string
	FQDN         string
	Addresses    []netip.Addr
	LastResolved time.Time // zero = never resolved
	NextRefresh  time.Time
	Err          string
	Failures     int
}

// fqdnEntry is the state of one host name (persisted).
type fqdnEntry struct {
	V4           []netip.Addr `json:"v4,omitempty"`
	V6           []netip.Addr `json:"v6,omitempty"`
	LastResolved time.Time    `json:"lastResolved"`
	NextRefresh  time.Time    `json:"nextRefresh"`
	Err          string       `json:"error,omitempty"`
	Failures     int          `json:"failures,omitempty"`
	// DormantSince: no object uses the name since then; the answers are kept (not refreshed) for
	// DormantTTL, so an object that comes back — a rollback, the resync after a lost store —
	// gets them at once, without a query.
	DormantSince time.Time `json:"dormantSince,omitzero"`
}

func (e *fqdnEntry) addrs() []netip.Addr {
	return append(append([]netip.Addr{}, e.V4...), e.V6...)
}

type fqdnFile struct {
	Version int                   `json:"version"`
	Hosts   map[string]*fqdnEntry `json:"hosts"`
}

type resolver struct {
	lookup  Lookup
	refresh time.Duration
	now     func() time.Time
	log     *slog.Logger
	path    string

	mu      sync.Mutex
	hosts   map[string]*fqdnEntry // canonical host → state
	objects map[string]string     // FQDN object name → canonical host
	subs    map[int]func(Change)
	nextSub int
	queries int // lookups started (both families count once), for tests and logs

	wake chan struct{}
}

func newResolver(path string, lookup Lookup, refresh time.Duration, now func() time.Time, log *slog.Logger) *resolver {
	return &resolver{
		lookup: lookup, refresh: ClampRefresh(refresh), now: now, log: log, path: path,
		hosts: map[string]*fqdnEntry{}, objects: map[string]string{}, subs: map[int]func(Change){}, wake: make(chan struct{}, 1),
	}
}

// load reads the persisted state. It is a cache of answers: an unreadable file is logged and
// ignored (the names are resolved again), never fatal.
func (r *resolver) load() {
	raw, err := os.ReadFile(r.path) //nolint:gosec // agent state file in the configured state dir
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	var f fqdnFile
	if err == nil {
		err = json.Unmarshal(raw, &f)
	}
	if err != nil {
		r.log.Warn("fqdn state unreadable; names are resolved again", "file", r.path, "err", err)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for h, e := range f.Hosts {
		if e != nil {
			r.hosts[canonHost(h)] = e
		}
	}
}

// persist writes the state file (0600, atomic). A failure is logged: the answers stay in memory.
func (r *resolver) persist() {
	r.mu.Lock()
	raw, err := json.MarshalIndent(fqdnFile{Version: 1, Hosts: r.hosts}, "", "  ")
	r.mu.Unlock()
	if err == nil {
		err = atomicWrite(r.path, raw)
	}
	if err != nil {
		r.log.Error("persist fqdn state", "file", r.path, "err", err)
	}
}

func (r *resolver) poke() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// sync makes the resolver track exactly objs (FQDN object name → host). A new host is due now; a
// host no object uses any more becomes dormant (kept, not queried) and is dropped after DormantTTL.
// With initial (agent start) the due entries are spread over RestartWindow: persisted answers that
// are still fresh keep their next refresh, so a restart does not re-query everything at once.
func (r *resolver) sync(objs map[string]string, initial bool) {
	now := r.now()
	r.mu.Lock()
	r.objects = map[string]string{}
	used := map[string]bool{}
	for name, host := range objs {
		h := canonHost(host)
		r.objects[name] = h
		used[h] = true
	}
	changed := false
	for h := range used {
		if e, ok := r.hosts[h]; !ok {
			r.hosts[h] = &fqdnEntry{NextRefresh: now}
			changed = true
		} else if !e.DormantSince.IsZero() {
			e.DormantSince = time.Time{}
			changed = true
		}
	}
	changed = r.pruneLocked(now, used) || changed
	var fresh, due []string
	dormant := 0
	for h, e := range r.hosts {
		switch {
		case !used[h]:
			dormant++
		case e.NextRefresh.After(now):
			fresh = append(fresh, h)
		default:
			due = append(due, h)
		}
	}
	if initial && len(due) > 0 {
		sort.Strings(due)
		step := RestartWindow / time.Duration(len(due))
		for i, h := range due {
			r.hosts[h].NextRefresh = now.Add(time.Duration(i) * step)
		}
		changed = true
	}
	r.mu.Unlock()
	if initial {
		r.log.Info("fqdn state reloaded", "file", r.path, "hosts", len(fresh)+len(due), "fresh", len(fresh), "due", len(due),
			"dormant", dormant, "due_spread_over", RestartWindow.String())
	}
	if changed {
		r.persist()
	}
	r.poke()
}

// pruneLocked marks hosts outside used dormant and drops those dormant for longer than DormantTTL.
func (r *resolver) pruneLocked(now time.Time, used map[string]bool) bool {
	changed := false
	for h, e := range r.hosts {
		switch {
		case used[h]:
		case e.DormantSince.IsZero():
			e.DormantSince = now
			changed = true
		case now.Sub(e.DormantSince) > DormantTTL:
			delete(r.hosts, h)
			changed = true
		}
	}
	return changed
}

// usedLocked is the set of hosts some object uses.
func (r *resolver) usedLocked() map[string]bool {
	used := map[string]bool{}
	for _, h := range r.objects {
		used[h] = true
	}
	return used
}

// untilNext is the time until the earliest scheduled refresh (MaxRefresh when there is none).
func (r *resolver) untilNext() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	next := MaxRefresh
	used := r.usedLocked()
	for h, e := range r.hosts {
		if !used[h] {
			continue
		}
		if d := e.NextRefresh.Sub(now); d < next {
			next = d
		}
	}
	if next < 0 {
		return 0
	}
	return next
}

// resolveDue resolves the hosts whose refresh is due (earliest first), at most limit of them
// (limit ≤ 0: all), persists the result and returns how many it resolved.
func (r *resolver) resolveDue(ctx context.Context, limit int) int {
	now := r.now()
	r.mu.Lock()
	used := r.usedLocked()
	pruned := r.pruneLocked(now, used)
	var due []string
	for h, e := range r.hosts {
		if used[h] && !e.NextRefresh.After(now) {
			due = append(due, h)
		}
	}
	sort.Slice(due, func(i, j int) bool {
		a, b := r.hosts[due[i]].NextRefresh, r.hosts[due[j]].NextRefresh
		if !a.Equal(b) {
			return a.Before(b)
		}
		return due[i] < due[j]
	})
	r.mu.Unlock()
	if limit > 0 && len(due) > limit {
		due = due[:limit]
	}
	for _, h := range due {
		if ctx.Err() != nil {
			break
		}
		r.resolveOne(ctx, h)
	}
	if len(due) > 0 || pruned {
		r.persist()
	}
	return len(due)
}

type familyResult struct {
	addrs []netip.Addr
	ttl   time.Duration
	err   error
}

func (r *resolver) resolveOne(ctx context.Context, host string) {
	lctx, cancel := context.WithTimeout(ctx, LookupTimeout)
	defer cancel()
	r.mu.Lock()
	r.queries++
	r.mu.Unlock()
	var v4, v6 familyResult
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); v4.addrs, v4.ttl, v4.err = r.lookup.LookupFamily(lctx, "ip4", host+".") }()
	go func() { defer wg.Done(); v6.addrs, v6.ttl, v6.err = r.lookup.LookupFamily(lctx, "ip6", host+".") }()
	wg.Wait()

	now := r.now()
	r.mu.Lock()
	e, ok := r.hosts[host]
	if !ok { // dropped by a sync meanwhile
		r.mu.Unlock()
		return
	}
	before := e.addrs()
	ok4, ok6 := v4.err == nil || IsNotFound(v4.err), v6.err == nil || IsNotFound(v6.err)
	var failure string
	switch {
	case ok4 && ok6 && len(v4.addrs)+len(v6.addrs) > 0:
		e.V4, e.V6 = canonAddrs(v4.addrs), canonAddrs(v6.addrs)
		e.Err, e.Failures, e.LastResolved = "", 0, now
		e.NextRefresh = now.Add(r.interval(v4, v6))
	case ok4 && ok6: // both families answered: the name has no address (NXDOMAIN or no A/AAAA)
		failure = "no A or AAAA records"
		if v4.err != nil {
			failure = v4.err.Error()
		}
	case !ok4 && !ok6: // both failed (usually for the same reason): report the A lookup's error
		failure = v4.err.Error()
	default: // one family answered, the other failed: use the answer, keep the other's last-good
		if ok4 {
			e.V4 = canonAddrs(v4.addrs)
			failure = v6.err.Error()
		} else {
			e.V6 = canonAddrs(v6.addrs)
			failure = v4.err.Error()
		}
		e.LastResolved = now
	}
	if failure != "" {
		e.Err = failure
		e.Failures++
		e.NextRefresh = now.Add(r.retryDelay(e.Failures))
	}
	after := e.addrs()
	st := *e
	objs := r.objectsOf(host)
	var subs []func(Change)
	changed := !slices.Equal(before, after)
	if changed {
		for _, id := range sortedIDs(r.subs) {
			subs = append(subs, r.subs[id])
		}
	}
	r.mu.Unlock()

	switch {
	case failure != "":
		r.log.Warn("fqdn resolution failed; last-good addresses kept", "host", host, "objects", objs, "err", failure,
			"kept", addrStrings(after), "failures", st.Failures, "retry_at", st.NextRefresh.UTC().Format(time.RFC3339))
	case changed:
		r.log.Info("fqdn resolved", "host", host, "objects", objs, "addresses", addrStrings(after), "next_refresh", st.NextRefresh.UTC().Format(time.RFC3339))
	default:
		r.log.Debug("fqdn refreshed, unchanged", "host", host, "addresses", addrStrings(after), "next_refresh", st.NextRefresh.UTC().Format(time.RFC3339))
	}
	for _, f := range subs {
		f(Change{Host: host, Objects: objs, Addresses: after})
	}
}

// interval is the next refresh after a success: the smallest TTL a family reported, clamped, or
// the fixed interval when there is none.
func (r *resolver) interval(rs ...familyResult) time.Duration {
	var ttl time.Duration
	for _, x := range rs {
		if x.err == nil && x.ttl > 0 && (ttl == 0 || x.ttl < ttl) {
			ttl = x.ttl
		}
	}
	if ttl == 0 {
		return r.refresh
	}
	return ClampRefresh(ttl)
}

// retryDelay after n consecutive failures: MinRefresh doubling, at most the refresh interval.
func (r *resolver) retryDelay(n int) time.Duration {
	d := MinRefresh
	for i := 1; i < n && d < r.refresh; i++ {
		d *= 2
	}
	if d > r.refresh {
		d = r.refresh
	}
	return d
}

func (r *resolver) objectsOf(host string) []string {
	var out []string
	for n, h := range r.objects {
		if h == host {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// addresses returns the addresses in use for FQDN object name; false when the object is unknown
// or has none.
func (r *resolver) addresses(name string) ([]netip.Addr, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.objects[name]
	if !ok {
		return nil, false
	}
	e := r.hosts[h]
	if e == nil {
		return nil, false
	}
	a := e.addrs()
	return a, len(a) > 0
}

// states returns the per-object view, sorted by object name (names filters; unknown are skipped).
func (r *resolver) states(names []string) []FQDNState {
	r.mu.Lock()
	defer r.mu.Unlock()
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	var out []FQDNState
	for n, h := range r.objects {
		if len(want) > 0 && !want[n] {
			continue
		}
		st := FQDNState{Name: n, FQDN: h}
		if e := r.hosts[h]; e != nil {
			st.Addresses, st.LastResolved, st.NextRefresh, st.Err, st.Failures = e.addrs(), e.LastResolved, e.NextRefresh, e.Err, e.Failures
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *resolver) subscribe(f func(Change)) func() {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.nextSub
	r.nextSub++
	r.subs[id] = f
	return func() {
		r.mu.Lock()
		delete(r.subs, id)
		r.mu.Unlock()
	}
}

// run is the resolver loop: resolve what is due (rate limited), sleep until the next refresh or a
// sync. It returns when ctx ends.
func (r *resolver) run(ctx context.Context) {
	for {
		n := r.resolveDue(ctx, queryBurst)
		wait := r.untilNext()
		if n == queryBurst && wait < querySpacing {
			wait = querySpacing
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-r.wake:
			t.Stop()
		case <-t.C:
		}
	}
}

func canonAddrs(in []netip.Addr) []netip.Addr {
	out := make([]netip.Addr, 0, len(in))
	for _, a := range in {
		if a.IsValid() {
			out = append(out, a.WithZone(""))
		}
	}
	slices.SortFunc(out, func(a, b netip.Addr) int { return a.Compare(b) })
	return slices.Compact(out)
}

func addrStrings(as []netip.Addr) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.String()
	}
	return out
}

func sortedIDs(m map[int]func(Change)) []int {
	out := make([]int, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}
