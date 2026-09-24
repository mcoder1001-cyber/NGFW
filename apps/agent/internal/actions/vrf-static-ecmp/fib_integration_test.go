package vrfstaticecmp_test

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	vse "ngfw/agent/internal/actions/vrf-static-ecmp"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestFIBBrowser100kOnHost (VRX_INTEGRATION=1, shared lab lock): 100 000 prefixes inside 10.<slot>.0.0/16 in one table
// of the slot's range (base+100, named "<prefix>f:fib"), then ListRoutes pages of 1000: the timing of each page and the
// size of the gRPC answer (only the page) are logged. Cleanup removes the routes first, then the table (V15). No tuning:
// FAST MODE records the timing only.
func TestFIBBrowser100kOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	restarts0 := nRestarts(t)
	t.Cleanup(func() {
		if n := nRestarts(t); n != restarts0 {
			t.Errorf("VPP restarted during the test: NRestarts %s → %s", restarts0, n)
		}
	})
	slot := vpptest.Slot(t)
	table := vpptest.TableBase(t) + 100
	name := vpptest.Prefix(t) + "f:fib"
	c := dialVPP(t)
	ctx := context.Background()
	cl := ip.NewServiceClient(c)

	prefixes := slotPrefixes(slot, 100_000)
	drop := []fib_types.FibPath{{SwIfIndex: ^uint32(0), TableID: table, Type: fib_types.FIB_API_PATH_TYPE_DROP, Proto: fib_types.FIB_API_PATH_NH_PROTO_IP4, Weight: 1}}
	send := func(p netip.Prefix, add bool) error {
		pfx, err := ip_types.ParsePrefix(p.String())
		if err != nil {
			return err
		}
		r := ip.IPRoute{TableID: table, Prefix: pfx}
		if add {
			r.NPaths, r.Paths = 1, drop
		}
		_, err = cl.IPRouteAddDel(ctx, &ip.IPRouteAddDel{IsAdd: add, Route: r})
		return err
	}
	// cleanup: every API route of the table, re-dumped until none is left (a route left behind would leak into the next
	// table that reuses the FIB index, V15), then the table — never the table while routes remain.
	cleanup := func() {
		start, n := time.Now(), 0
		for pass := 0; pass < 5; pass++ {
			var mine []netip.Prefix
			err := vse.DumpRoutes(ctx, c, &ip.IPRouteV2Dump{Src: 8, Table: ip.IPTable{TableID: table}}, func(d *ip.IPRouteV2Details) {
				if p, err := netip.ParsePrefix(d.Route.Prefix.String()); err == nil {
					mine = append(mine, p)
				}
			})
			if err != nil {
				t.Errorf("cleanup dump: %v", err)
				return
			}
			if len(mine) == 0 {
				_, _ = cl.IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: false, Table: ip.IPTable{TableID: table, Name: name}})
				t.Logf("cleanup: %d routes deleted in %d pass(es), then table %d, in %v", n, pass, table, time.Since(start))
				return
			}
			for _, p := range mine {
				if err := send(p, false); err == nil {
					n++
				}
			}
		}
		t.Errorf("cleanup: API routes still in table %d after 5 passes; table NOT deleted (V15)", table)
	}
	if tableExists(t, cl, table) {
		if n := tableName(t, cl, table); n != name {
			t.Fatalf("table %d exists as %q, not ours", table, n)
		}
		cleanup()
	}
	if _, err := cl.IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: true, Table: ip.IPTable{TableID: table, Name: name}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	start := time.Now()
	for i, p := range prefixes {
		if err := send(p, true); err != nil {
			t.Fatalf("add #%d %s: %v", i, p, err)
		}
	}
	t.Logf("injected %d prefixes into table %d in %v", len(prefixes), table, time.Since(start))

	for _, q := range []vse.Query{
		{Table: table, Limit: 1000},
		{Table: table, Offset: 50_000, Limit: 1000},
		{Table: table, Offset: 99_000, Limit: 1000},
		{Table: table, Prefix: fmt.Sprintf("10.%d.200.0/24", slot), Limit: 1000},
		{Table: table, Source: "API", Offset: 1000, Limit: 1000},
	} {
		start := time.Now()
		page, err := vse.ListRoutes(ctx, c, vpptest.Prefix(t), q)
		took := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		resp := &vrxv1.ListRoutesResponse{Routes: page.Routes, Total: page.Total, TableId: table, RetrievedAt: timestamppb.Now()}
		t.Logf("ListRoutes offset=%d limit=%d prefix=%q source=%q: %d routes of total %d in %v; gRPC answer %d bytes (first %s, last %s)",
			q.Offset, q.Limit, q.Prefix, q.Source, len(page.Routes), page.Total, took.Round(time.Millisecond), proto.Size(resp),
			first(page), last(page))
		if q.Prefix == "" && q.Source == "" && page.Total < 100_000 {
			t.Errorf("total %d < 100000", page.Total)
		}
		if len(page.Routes) > 1000 {
			t.Errorf("page of %d routes", len(page.Routes))
		}
	}
}

// slotPrefixes returns n distinct IPv4 prefixes inside 10.<slot>.0.0/16: every /32, then every /31, then /30s.
func slotPrefixes(slot, n int) []netip.Prefix {
	base := netip.MustParseAddr(fmt.Sprintf("10.%d.0.0", slot)).As4()
	out := make([]netip.Prefix, 0, n)
	for bits := 32; bits >= 24 && len(out) < n; bits-- {
		step := 1 << (32 - bits)
		for i := 0; i < 65536 && len(out) < n; i += step {
			a := base
			a[2], a[3] = byte(i>>8), byte(i)
			out = append(out, netip.PrefixFrom(netip.AddrFrom4(a), bits))
		}
	}
	return out
}

func first(p *vse.Page) string {
	if len(p.Routes) == 0 {
		return "-"
	}
	return p.Routes[0].GetPrefix()
}

func last(p *vse.Page) string {
	if len(p.Routes) == 0 {
		return "-"
	}
	return p.Routes[len(p.Routes)-1].GetPrefix()
}

func tableExists(t *testing.T, cl ip.RPCService, id uint32) bool { return tableName(t, cl, id) != "" }

func tableName(t *testing.T, cl ip.RPCService, id uint32) string {
	t.Helper()
	stream, err := cl.IPTableDump(context.Background(), &ip.IPTableDump{})
	if err != nil {
		t.Fatal(err)
	}
	name := ""
	for {
		d, err := stream.Recv()
		if err != nil {
			return name
		}
		if d.Table.TableID == id && !d.Table.IsIP6 {
			name = strings.TrimRight(d.Table.Name, "\x00")
		}
	}
}

func nRestarts(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("systemctl", "show", "vpp", "-p", "NRestarts", "--value").Output()
	if err != nil {
		t.Fatalf("systemctl show vpp: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func dialVPP(t *testing.T) *vpp.Conn {
	t.Helper()
	path := os.Getenv("VRX_VPP_API_SOCKET")
	if path == "" {
		path = "/run/vpp/api.sock"
	}
	c := vpp.Dial(path, vpp.ConnOptions{})
	t.Cleanup(c.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := c.WaitConnected(ctx); err != nil {
		t.Fatal(err)
	}
	return c
}
