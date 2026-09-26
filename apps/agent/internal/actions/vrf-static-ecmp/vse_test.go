package vrfstaticecmp_test

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	vse "ngfw/agent/internal/actions/vrf-static-ecmp"
	"ngfw/agent/internal/descriptors/core/coretest"
)

// fibModel: table 2100 with n /32 API routes 10.2.x.y (x.y from i), a /24 with two weighted paths and an IPv6 route.
func fibModel(t *testing.T, n int) *coretest.VPP {
	t.Helper()
	v := coretest.New().InstallVrfStaticEcmp()
	c := ip.NewServiceClient(v)
	ctx := context.Background()
	for _, v6 := range []bool{false, true} {
		if _, err := c.IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: true, Table: ip.IPTable{TableID: 2100, IsIP6: v6, Name: "w2:fib"}}); err != nil {
			t.Fatal(err)
		}
	}
	add := func(prefix string, paths ...fib_types.FibPath) {
		p, err := ip_types.ParsePrefix(prefix)
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) == 0 {
			paths = []fib_types.FibPath{{SwIfIndex: ^uint32(0), Type: fib_types.FIB_API_PATH_TYPE_DROP, Weight: 1}}
		}
		if _, err := c.IPRouteAddDel(ctx, &ip.IPRouteAddDel{IsAdd: true, Route: ip.IPRoute{TableID: 2100, Prefix: p, NPaths: uint8(len(paths)), Paths: paths}}); err != nil { //nolint:gosec // test
			t.Fatal(err)
		}
	}
	for i := 0; i < n; i++ {
		add(fmt.Sprintf("10.2.%d.%d/32", 100+i/256, i%256))
	}
	nh := func(a string, w uint8) fib_types.FibPath {
		ad := netip.MustParseAddr(a)
		return fib_types.FibPath{SwIfIndex: ^uint32(0), TableID: 2100, Weight: w, Proto: fib_types.FIB_API_PATH_NH_PROTO_IP4,
			Nh: fib_types.FibPathNh{Address: ip_types.AddressUnionIP4(ip_types.IP4Address(ad.As4()))}}
	}
	add("10.2.9.0/24", nh("10.2.1.2", 3), nh("10.2.1.3", 1))
	add("2001:db8:2::/48")
	v.AddInternalRoute(2100, "10.2.1.0/24", 4) // a connected entry (source "interface")
	return v
}

func TestListRoutesPagesSortedAndBounded(t *testing.T) {
	v := fibModel(t, 2500)
	ctx := context.Background()
	all, err := vse.ListRoutes(ctx, v, "w2", vse.Query{Table: 2100, Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	// 2500 /32 + the /24 + the connected /24 + the IPv6 /48 + the recursive-resolution /32s of the ECMP next hops
	if all.Total < 2503 || len(all.Routes) != 1000 {
		t.Fatalf("total %d, page %d", all.Total, len(all.Routes))
	}
	var got []netip.Prefix
	for _, r := range all.Routes {
		got = append(got, netip.MustParsePrefix(r.GetPrefix()))
	}
	if !sort.SliceIsSorted(got, func(i, j int) bool {
		if got[i].Addr() != got[j].Addr() {
			return got[i].Addr().Less(got[j].Addr())
		}
		return got[i].Bits() < got[j].Bits()
	}) {
		t.Fatal("page not sorted")
	}
	// pages are consecutive: page 2 starts right after page 1
	p2, err := vse.ListRoutes(ctx, v, "w2", vse.Query{Table: 2100, Offset: 1000, Limit: 1000})
	if err != nil || p2.Total != all.Total {
		t.Fatalf("page 2: %v total %d", err, p2.Total)
	}
	if !netip.MustParsePrefix(all.Routes[999].GetPrefix()).Addr().Less(netip.MustParsePrefix(p2.Routes[0].GetPrefix()).Addr()) {
		t.Fatalf("page 2 starts at %s after %s", p2.Routes[0].GetPrefix(), all.Routes[999].GetPrefix())
	}
	// last page: IPv6 comes after every IPv4 route
	last, err := vse.ListRoutes(ctx, v, "w2", vse.Query{Table: 2100, Offset: all.Total - 1, Limit: 10})
	if err != nil || len(last.Routes) != 1 || last.Routes[0].GetPrefix() != "2001:db8:2::/48" {
		t.Fatalf("last page %v %v", err, last.Routes)
	}
	// beyond the end: empty page, total still reported
	if past, err := vse.ListRoutes(ctx, v, "w2", vse.Query{Table: 2100, Offset: all.Total + 5}); err != nil || len(past.Routes) != 0 || past.Total != all.Total {
		t.Fatalf("past the end %v %+v", err, past)
	}
}

func TestListRoutesFiltersAndDetail(t *testing.T) {
	v := fibModel(t, 300)
	ctx := context.Background()
	within, err := vse.ListRoutes(ctx, v, "w2", vse.Query{Table: 2100, Prefix: "10.2.9.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	if within.Total != 1 || within.Routes[0].GetPrefix() != "10.2.9.0/24" {
		t.Fatalf("within 10.2.9.0/24: %+v", within)
	}
	ecmp := within.Routes[0]
	if ecmp.GetSource() != "API" || len(ecmp.GetPaths()) != 2 {
		t.Fatalf("ECMP entry %+v", ecmp)
	}
	w := []string{}
	for _, p := range ecmp.GetPaths() {
		w = append(w, fmt.Sprintf("%s/%s/%d/%d", p.GetType(), p.GetNextHop(), p.GetWeight(), p.GetTableId()))
	}
	if strings.Join(w, " ") != "normal/10.2.1.2/3/2100 normal/10.2.1.3/1/2100" {
		t.Fatalf("paths %v", w)
	}
	v6, err := vse.ListRoutes(ctx, v, "w2", vse.Query{Table: 2100, Family: "ipv6"})
	if err != nil || v6.Total != 1 {
		t.Fatalf("ipv6: %v %+v", err, v6)
	}
	conn, err := vse.ListRoutes(ctx, v, "w2", vse.Query{Table: 2100, Source: "interface"})
	if err != nil || conn.Total != 1 || conn.Routes[0].GetPrefix() != "10.2.1.0/24" || conn.Routes[0].GetSource() != "interface" {
		t.Fatalf("source=interface: %v %+v", err, conn)
	}
	for _, bad := range []vse.Query{
		{Table: 2100, Source: "no-such-source"},
		{Table: 2100, Prefix: "10.2.0.0/33"},
		{Table: 2100, Prefix: "10.2.0.0/16", Family: "ipv6"},
		{Table: 2100, Family: "ip4"},
		{Table: 2100, Limit: 1001},
		{Table: 2100, Offset: vse.MaxWindow},
		{Table: 2100, Offset: vse.MaxWindow - 999, Limit: 1000},
	} {
		if _, err := vse.ListRoutes(ctx, v, "w2", bad); !errors.Is(err, vse.ErrBadRequest) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	// the deepest page the window allows is accepted
	if deep, err := vse.ListRoutes(ctx, v, "w2", vse.Query{Table: 2100, Offset: vse.MaxWindow - 1000, Limit: 1000}); err != nil || len(deep.Routes) != 0 {
		t.Fatalf("deepest page: %v %+v", err, deep)
	}
	// a table that does not exist is an empty FIB, not an error
	if none, err := vse.ListRoutes(ctx, v, "w2", vse.Query{Table: 2999}); err != nil || none.Total != 0 {
		t.Fatalf("missing table %v %+v", err, none)
	}
}

func TestPingValidation(t *testing.T) {
	ok, err := vse.ValidatePing(&vrxv1.PingAction{Target: "10.2.2.2"})
	if err != nil || ok.Count != 5 || ok.Interval != time.Second {
		t.Fatalf("defaults %+v %v", ok, err)
	}
	for _, bad := range []*vrxv1.PingAction{
		{Target: "vrx.example"},
		{Target: "10.2.2.2", Vrf: "red"},
		{Target: "10.2.2.2", Source: "10.2.1.1"},
		{Target: "10.2.2.2", Size: 1400},
		{Target: "10.2.2.2", Count: 101},
		{Target: "10.2.2.2", IntervalMs: 50},
		{Target: "10.2.2.2", Count: 10, IntervalMs: 1000}, // 10 s holds VPP's API too long
	} {
		if _, err := vse.ValidatePing(bad); !errors.Is(err, vse.ErrInvalid) {
			t.Errorf("%v: %v", bad, err)
		}
	}
	if _, err := vse.ValidatePing(&vrxv1.PingAction{Target: "10.2.2.2", Vrf: "default", Count: 20, IntervalMs: 250}); err != nil {
		t.Fatalf("20 × 250 ms in default: %v", err)
	}
	if err := vse.Traceroute(&vrxv1.TracerouteAction{Target: "10.2.2.2"}); !errors.Is(err, vse.ErrUnimplemented) || !strings.Contains(err.Error(), vse.VItem) {
		t.Fatalf("traceroute %v", err)
	}
}

func TestPingSummaryAndStats(t *testing.T) {
	v := coretest.New().InstallVrfStaticEcmp()
	v.Svs().PingResult = func(a netip.Addr, repeat uint32) (uint32, uint32) {
		if a.String() != "10.2.2.2" {
			return repeat, 0
		}
		return repeat, repeat - 1
	}
	plan, err := vse.ValidatePing(&vrxv1.PingAction{Target: "10.2.2.2", Count: 4, IntervalMs: 100})
	if err != nil {
		t.Fatal(err)
	}
	var out []*vrxv1.ActionOutput
	if err := vse.Ping(context.Background(), v, plan, func(o *vrxv1.ActionOutput) error { out = append(out, o); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || !strings.Contains(out[0].GetLine(), "4 packets transmitted, 3 received, 25% packet loss") {
		t.Fatalf("output %v", out)
	}
	d := out[1].GetDone()
	if d.GetExitCode() != 0 || d.GetStats()["transmitted"] != "4" || d.GetStats()["received"] != "3" || d.GetStats()["loss_pct"] != "25" {
		t.Fatalf("done %v", d)
	}
	sent := v.Svs().Pings[0]
	if sent.Repeat != 4 || sent.Interval != 0.1 {
		t.Fatalf("request %+v", sent)
	}
	// no reply → exit code 1
	plan.Target = netip.MustParseAddr("10.2.2.9")
	out = nil
	if err := vse.Ping(context.Background(), v, plan, func(o *vrxv1.ActionOutput) error { out = append(out, o); return nil }); err != nil {
		t.Fatal(err)
	}
	if out[1].GetDone().GetExitCode() != 1 {
		t.Fatalf("no replies: %v", out[1])
	}
}

// Review H1 (c): N concurrent ListRoutes calls → at most one ip_route_v2_dump in flight (VPP runs the dump under its
// worker barrier; the agent serialises walks), and every caller still gets its page.
func TestListRoutesOneWalkAtATime(t *testing.T) {
	v := fibModel(t, 10)
	var inflight, peak, walks atomic.Int32
	v.On("ip_route_v2_dump", func(api.Message) ([]api.Message, error) {
		n := inflight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		walks.Add(1)
		inflight.Add(-1)
		return nil, nil
	})
	const callers = 6
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := vse.ListRoutes(context.Background(), v, "w2", vse.Query{Table: 2100, Limit: 10})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("caller: %v", err)
		}
	}
	if peak.Load() != 1 || walks.Load() != 2*callers {
		t.Fatalf("peak in-flight dumps %d (want 1), dumps %d (want %d)", peak.Load(), walks.Load(), 2*callers)
	}
}

// A caller that cannot start its walk in time gets ErrFIBBusy (→ UNAVAILABLE); the walk in progress is unaffected.
func TestListRoutesBusyWhileAWalkRuns(t *testing.T) {
	v := fibModel(t, 10)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	v.On("ip_route_v2_dump", func(api.Message) ([]api.Message, error) {
		once.Do(func() { close(started) })
		<-release
		return nil, nil
	})
	first := make(chan error, 1)
	go func() {
		_, err := vse.ListRoutes(context.Background(), v, "w2", vse.Query{Table: 2100, Family: "ipv4"})
		first <- err
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := vse.ListRoutes(ctx, v, "w2", vse.Query{Table: 2100}); !errors.Is(err, vse.ErrFIBBusy) {
		t.Fatalf("second walk while the first runs: %v", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatalf("first walk: %v", err)
	}
	if _, err := vse.ListRoutes(context.Background(), v, "w2", vse.Query{Table: 2100}); err != nil {
		t.Fatalf("after the walk: %v", err)
	}
}

// Review M1: VPP's ping API is not mp-safe, so a VPP with worker threads refuses the ping before anything is sent.
func TestPingRefusedWithWorkerThreads(t *testing.T) {
	v := coretest.New().InstallVrfStaticEcmp()
	v.Svs().Workers = 1
	plan, err := vse.ValidatePing(&vrxv1.PingAction{Target: "10.2.2.2", Count: 2, IntervalMs: 100})
	if err != nil {
		t.Fatal(err)
	}
	err = vse.Ping(context.Background(), v, plan, func(*vrxv1.ActionOutput) error { return nil })
	if !errors.Is(err, vse.ErrWorkers) || !strings.Contains(err.Error(), "worker barrier") || !strings.Contains(err.Error(), vse.VItem) {
		t.Fatalf("ping with a worker thread: %v", err)
	}
	if n := len(v.Svs().Pings); n != 0 {
		t.Fatalf("%d ping request(s) reached VPP", n)
	}
}
