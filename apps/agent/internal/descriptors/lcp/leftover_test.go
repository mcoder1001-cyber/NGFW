package lcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/lcp"
	"ngfw/agent/binapi/mfib_types"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
)

// TD-lcp-leftover-local-path (S-lcp-netns-224-accept Q2, arbiter finding 2): a lone API local
// path left in the (*,224.0.0.0/24) of a table is removed before a default-namespace pair is added
// there; an API source that still serves a pair outside the lcp default namespace in that table
// refuses it. Fix round 1 (review finding 1, option (a)): an Accept on the phy of such a pair that
// is in another table now is deleted (VPP ignores the delete for a stale linux-cp Accept), never
// refused.

// mrouteOrder lists the mroute and pair add/del requests in call order.
func mrouteOrder(t *testing.T, calls []api.Message) []string {
	t.Helper()
	var out []string
	for _, c := range calls {
		switch r := c.(type) {
		case *ip.IPMrouteAddDel:
			for _, p := range r.Route.Paths {
				what := fmt.Sprintf("accept %d", p.Path.SwIfIndex)
				if p.Path.Type == fib_types.FIB_API_PATH_TYPE_LOCAL {
					what = "local"
				}
				out = append(out, fmt.Sprintf("mroute add=%v table %d %s", r.IsAdd, r.Route.TableID, what))
			}
		case *lcp.LcpItfPairAddDelV3:
			out = append(out, fmt.Sprintf("pair add=%v %d", r.IsAdd, r.SwIfIndex))
		}
	}
	return out
}

// onDelete wraps the model's ip_mroute_add_del: hook runs once, right after the first delete of a
// path on sw_if_index idx (^0: the local path) was applied.
func onDelete(f *dfkittest.FakeVPP, m *model, idx uint32, hook func()) {
	f.On("ip_mroute_add_del", func(msg api.Message) ([]api.Message, error) {
		out, err := m.mroute(msg)
		r := msg.(*ip.IPMrouteAddDel)
		if hook != nil && err == nil && !r.IsAdd && slices.ContainsFunc(r.Route.Paths, func(p mfib_types.MfibPath) bool {
			return p.Path.SwIfIndex == idx
		}) {
			h := hook
			hook = nil
			h()
		}
		return out, err
	})
}

// w6NetnsPair is w6's pair on loop601 (sw_if_index 8) in namespace ns-w6, outside the lcp default
// namespace.
func w6NetnsPair() lcp.LcpItfPairDetails {
	return lcp.LcpItfPairDetails{PhySwIfIndex: 8, HostIfName: "w6-lcp0", HostIfType: lcp.LCP_API_ITF_HOST_TAP, Netns: "ns-w6"}
}

// w5DefaultPair is the default-namespace pair every test here creates on loop501 (sw_if_index 7).
var w5DefaultPair = ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap"}

// The leftover case: table 5 (a VRF linux-cp has no router table for) holds only the API source's
// local path. Create deletes it, then adds the pair, and linux-cp's Accept for the new pair is
// forwarded.
func TestDefaultPairClearsLeftoverLocal(t *testing.T) {
	f, m := newFake()
	m.noRouter = true
	m.table[7] = 5
	m.apiAccept[^uint32(0)] = 1 // Q2: kept after the last Accept went (dump error or race)
	d := NewItfPair(f, "w5")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap"}.Proto()
	if _, err := d.Create(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	if len(m.apiAccept) != 0 {
		t.Fatalf("leftover kept: API paths %v", m.apiAccept)
	}
	if got, want := mrouteOrder(t, f.Calls()), []string{"mroute add=false table 5 local", "pair add=true 7"}; !slices.Equal(got, want) {
		t.Fatalf("calls %q, want %q", got, want)
	}
	if paths, found, err := mroute224(context.Background(), f, 5); err != nil || found {
		t.Fatalf("entry after the delete: %v %v %v, want gone", paths, found, err)
	}
	m.lowAccept[7] = true // linux-cp hears the host address: its plugin-low Accept is the one forwarded
	if ok, err := acceptIn(context.Background(), f, 5, 7); err != nil || !ok {
		t.Fatalf("plugin-low Accept of the new pair not forwarded: %v %v", ok, err)
	}
}

// The leftover under linux-cp's own source (table 0 style: router-table local path plus the
// Accept of w6's default-namespace pair, both shadowed by the lone API source): after the delete
// linux-cp's view is forwarded again and the pair is added.
func TestLeftoverLocalOverLinuxCP(t *testing.T) {
	f, m := newFake()
	m.pairs[8] = lcp.LcpItfPairDetails{PhySwIfIndex: 8, HostIfName: "w6-lcp0", HostIfType: lcp.LCP_API_ITF_HOST_TAP}
	m.lowAccept[8] = true
	m.apiAccept[^uint32(0)] = 1
	if ok, _ := acceptIn(context.Background(), f, 0, 8); ok {
		t.Fatal("model: the lone API source must shadow linux-cp's Accept")
	}
	d := NewItfPair(f, "w5")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap"}.Proto()
	if _, err := d.Create(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	if got, want := mrouteOrder(t, f.Calls()), []string{"mroute add=false table 0 local", "pair add=true 7"}; !slices.Equal(got, want) {
		t.Fatalf("calls %q, want %q", got, want)
	}
	if ok, err := acceptIn(context.Background(), f, 0, 8); err != nil || !ok || len(m.apiAccept) != 0 {
		t.Fatalf("linux-cp's Accept of w6 not forwarded again: %v %v, API paths %v", ok, err, m.apiAccept)
	}
}

// The still-mixed case: the API source serves a pair outside the lcp default namespace in the same
// table — refused by the pair check, as today. Nothing is touched or added. (The other round-1
// subtest, "netns pair moved, its Accept stays", is no refusal since fix round 1:
// TestDefaultPairDropsMovedPairAccept.)
func TestDefaultPairStillMixedRefused(t *testing.T) {
	t.Run("netns pair in the table", func(t *testing.T) {
		f, m := newFake()
		m.pairs[8] = w6NetnsPair()
		m.apiAccept[8], m.apiAccept[^uint32(0)] = 1, 1 // w6's Accept and the shared local path
		d := NewItfPair(f, "w5")
		_, err := d.Create(context.Background(), w5DefaultPair.Proto())
		if want := "already holds the pair of sw_if_index 8"; err == nil || !strings.Contains(err.Error(), "D-217") || !strings.Contains(err.Error(), want) {
			t.Fatalf("create: %v, want a D-217 refusal naming %q", err, want)
		}
		if got := mrouteOrder(t, f.Calls()); len(got) != 0 {
			t.Fatalf("refusal changed state: %q", got)
		}
		if m.apiAccept[8] != 1 || m.apiAccept[^uint32(0)] != 1 {
			t.Fatalf("w6's API source touched: %v", m.apiAccept)
		}
	})
}

// Review finding 1 — the reviewer's repro (TestReviewV7ReuseFalseRefusal in the review file, renamed
// for NIT 5; body verbatim, assertions added after it): linux-cp's stale Accept on sw_if_index 8 in
// table 0 while index 8 now carries a netns pair in VRF 6, no API path anywhere, must not refuse a
// default-namespace pair in table 0.
func TestReviewReuseFalseRefusal(t *testing.T) {
	f, m := newFake()
	m.pairs[8] = lcp.LcpItfPairDetails{PhySwIfIndex: 8, HostIfName: "w6-lcp0", HostIfType: lcp.LCP_API_ITF_HOST_TAP, Netns: "ns-w6"}
	m.table[8] = 6        // the netns pair lives in VRF 6
	m.lowAccept[8] = true // linux-cp's stale Accept on index 8 in table 0; no API path anywhere
	d := NewItfPair(f, "w5")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap"}.Proto()
	if _, err := d.Create(context.Background(), v); err != nil {
		t.Fatalf("FALSE REFUSAL (no API source in table 0): %v", err)
	}
	// only the API delete of index 8's Accept (VPP ignores it: no API source), then the pair;
	// linux-cp's stale Accept and w6's pair stay as they were
	if got, want := mrouteOrder(t, f.Calls()), []string{"mroute add=false table 0 accept 8", "pair add=true 7"}; !slices.Equal(got, want) {
		t.Fatalf("calls %q, want %q", got, want)
	}
	if !m.lowAccept[8] || len(m.apiAccept) != 0 {
		t.Fatalf("linux-cp's view changed: plugin-low %v, API %v", m.lowAccept, m.apiAccept)
	}
	if _, ok := m.pairs[8]; !ok {
		t.Fatal("w6's pair touched")
	}
}

// Review finding 1, option (a) — the adjusted "moved" subtest: w6's netns pair moved to VRF 6
// while its API Accept and the shared local path stayed in table 0. That Accept serves no packet
// (mfib looks a packet up in its input interface's own table), so Create deletes it, reads the entry
// and then the pairs again, deletes the now-lone API local path and adds the pair.
func TestDefaultPairDropsMovedPairAccept(t *testing.T) {
	f, m := newFake()
	m.pairs[8] = w6NetnsPair()
	m.table[8] = 6
	m.apiAccept[8], m.apiAccept[^uint32(0)] = 1, 1
	d := NewItfPair(f, "w5")
	if _, err := d.Create(context.Background(), w5DefaultPair.Proto()); err != nil {
		t.Fatal(err)
	}
	want := []string{"mroute add=false table 0 accept 8", "mroute add=false table 0 local", "pair add=true 7"}
	if got := mrouteOrder(t, f.Calls()); !slices.Equal(got, want) {
		t.Fatalf("calls %q, want %q", got, want)
	}
	if len(m.apiAccept) != 0 {
		t.Fatalf("API source left in table 0: %v", m.apiAccept)
	}
	if _, ok := m.pairs[8]; !ok {
		t.Fatal("w6's pair touched")
	}
	// after the Accept delete the entry is read before the pairs
	calls := f.Calls()
	i := slices.IndexFunc(calls, func(c api.Message) bool { _, ok := c.(*ip.IPMrouteAddDel); return ok })
	var after []string
	for _, c := range calls[i+1:] {
		after = append(after, c.GetMessageName())
	}
	dump, pairs := slices.Index(after, "ip_mroute_dump"), slices.Index(after, "lcp_itf_pair_get")
	if dump < 0 || pairs < 0 || dump > pairs {
		t.Fatalf("requests after the Accept delete %q: want the entry re-read before the pairs", after)
	}
}

// Option (a)'s re-read takes the entry before the pairs. Another agent creating a netns pair in
// table 0 adds the pair, then its Accept; here both land just before our re-read of the entry, so
// the pairs read after it know the pair and the default-namespace pair is refused (D-217), with the
// other agent's Accept and local path untouched. Reading the pairs first would miss the pair and
// take its Accept for one without a pair.
func TestMovedAcceptReReadSeesNewNetnsPair(t *testing.T) {
	f, m := newFake()
	m.pairs[8] = w6NetnsPair()
	m.table[8] = 6
	m.apiAccept[8], m.apiAccept[^uint32(0)] = 1, 1
	deleted := false
	onDelete(f, m, 8, func() { deleted = true })
	f.On("ip_mroute_dump", func(msg api.Message) ([]api.Message, error) {
		if _, ok := m.pairs[10]; deleted && !ok { // w7's Create: the pair, then its Accept (+ local)
			m.pairs[10] = lcp.LcpItfPairDetails{PhySwIfIndex: 10, HostIfName: "w7-lcp0", HostIfType: lcp.LCP_API_ITF_HOST_TAP, Netns: "ns-w7"}
			m.apiAccept[10], m.apiAccept[^uint32(0)] = 1, 1
		}
		return m.dump224(msg)
	})
	d := NewItfPair(f, "w5")
	_, err := d.Create(context.Background(), w5DefaultPair.Proto())
	if want := "already holds the API (*,224.0.0.0/24) Accept of the pair of sw_if_index 10"; err == nil ||
		!strings.Contains(err.Error(), "D-217") || !strings.Contains(err.Error(), want) {
		t.Fatalf("create: %v, want a D-217 refusal naming %q", err, want)
	}
	if got, want := mrouteOrder(t, f.Calls()), []string{"mroute add=false table 0 accept 8"}; !slices.Equal(got, want) {
		t.Fatalf("calls %q, want %q", got, want)
	}
	if m.apiAccept[10] != 1 || m.apiAccept[^uint32(0)] != 1 {
		t.Fatalf("w7's API source touched: %v", m.apiAccept)
	}
	if _, ok := m.pairs[7]; ok {
		t.Fatal("pair added despite the refusal")
	}
}

// linux-cp's own view is never refused and never touched: a stale Accept without a pair (stale
// linux-cp Accept, vpp-code-track lcp_router.c row, as in table 0 on ngfw-a), a default-namespace
// pair's Accept, or no entry at all. Its lone local path draws only the API local delete, a no-op
// for VPP (no API source).
func TestDefaultPairLeavesLinuxCPView(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(m *model)
		want  []string
	}{
		{"stale Accept without a pair", func(m *model) { m.lowAccept[1] = true }, nil},
		{"default-namespace pair's Accept", func(m *model) {
			m.pairs[8] = lcp.LcpItfPairDetails{PhySwIfIndex: 8, HostIfName: "w6-lcp0", HostIfType: lcp.LCP_API_ITF_HOST_TAP}
			m.lowAccept[8] = true
		}, nil},
		{"no entry", func(m *model) { m.noRouter = true }, nil},
		{"lone local path of linux-cp", func(*model) {}, []string{"mroute add=false table 0 local"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, m := newFake()
			tc.setup(m)
			d := NewItfPair(f, "w5")
			v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap"}.Proto()
			if _, err := d.Create(context.Background(), v); err != nil {
				t.Fatal(err)
			}
			if got, want := mrouteOrder(t, f.Calls()), append(tc.want, "pair add=true 7"); !slices.Equal(got, want) {
				t.Fatalf("calls %q, want %q", got, want)
			}
			if len(m.apiAccept) != 0 {
				t.Fatalf("API paths %v", m.apiAccept)
			}
		})
	}
}

// Race (Q2): another agent creates a pair outside the lcp default namespace in the table and adds
// its Accept between our dump and our delete, so the delete took its shared local path. Create
// gives the local path back and refuses (D-217); our pair is not added.
func TestLeftoverDeleteRace(t *testing.T) {
	f, m := newFake()
	m.noRouter = true
	m.apiAccept[^uint32(0)] = 1
	f.On("ip_mroute_add_del", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*ip.IPMrouteAddDel)
		for _, p := range r.Route.Paths {
			if r.IsAdd {
				m.apiAccept[p.Path.SwIfIndex] = 1
			} else {
				delete(m.apiAccept, p.Path.SwIfIndex)
			}
		}
		if _, ok := m.pairs[8]; !ok && !r.IsAdd { // meanwhile: w6's pair and Accept (its local path add was before our delete)
			m.pairs[8] = lcp.LcpItfPairDetails{PhySwIfIndex: 8, HostIfName: "w6-lcp0", HostIfType: lcp.LCP_API_ITF_HOST_TAP, Netns: "ns-w6"}
			m.apiAccept[8] = 1
		}
		return []api.Message{&ip.IPMrouteAddDelReply{}}, nil
	})
	d := NewItfPair(f, "w5")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap"}.Proto()
	_, err := d.Create(context.Background(), v)
	if err == nil || !strings.Contains(err.Error(), "D-217") {
		t.Fatalf("create: %v, want a D-217 refusal", err)
	}
	if got, want := mrouteOrder(t, f.Calls()), []string{"mroute add=false table 0 local", "mroute add=true table 0 local"}; !slices.Equal(got, want) {
		t.Fatalf("calls %q, want %q", got, want)
	}
	if m.apiAccept[8] != 1 || m.apiAccept[^uint32(0)] != 1 {
		t.Fatalf("w6's API source not restored: %v", m.apiAccept)
	}
	if _, ok := m.pairs[7]; ok {
		t.Fatal("pair added despite the refusal")
	}
}

// Review finding 2: when the check after the leftover local delete fails — the entry re-read, the
// pairs re-read, or a phy's table — Create gives the local path back before it returns the error
// (a racing netns pair's Accept would otherwise stay without it, and nothing repairs that); the pair
// is not added.
func TestLeftoverRestoresLocalOnReReadError(t *testing.T) {
	boom := errors.New("boom")
	race := func(m *model) { // meanwhile: w6's netns pair and its Accept in table 0
		m.pairs[8] = w6NetnsPair()
		m.apiAccept[8] = 1
	}
	for _, tc := range []struct {
		name string
		fail func(f *dfkittest.FakeVPP, m *model) // runs right after our local delete
	}{
		{"entry re-read fails", func(f *dfkittest.FakeVPP, _ *model) { f.Fail("ip_mroute_dump", boom) }},
		{"pairs re-read fails", func(f *dfkittest.FakeVPP, m *model) { race(m); f.Fail("lcp_itf_pair_get", boom) }},
		{"phy table read fails", func(f *dfkittest.FakeVPP, m *model) { race(m); f.Fail("sw_interface_get_table", boom) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, m := newFake()
			m.noRouter = true
			m.apiAccept[^uint32(0)] = 1
			onDelete(f, m, ^uint32(0), func() { tc.fail(f, m) })
			d := NewItfPair(f, "w5")
			if _, err := d.Create(context.Background(), w5DefaultPair.Proto()); !errors.Is(err, boom) {
				t.Fatalf("create: %v, want the read error", err)
			}
			if got, want := mrouteOrder(t, f.Calls()), []string{"mroute add=false table 0 local", "mroute add=true table 0 local"}; !slices.Equal(got, want) {
				t.Fatalf("calls %q, want %q", got, want)
			}
			if m.apiAccept[^uint32(0)] != 1 {
				t.Fatalf("local path not given back: %v", m.apiAccept)
			}
			if _, ok := m.pairs[7]; ok {
				t.Fatal("pair added despite the error")
			}
		})
	}
}

// A failed recovery must keep both errors observable and never create the pair.
func TestLeftoverRestoreFailureKeepsBothErrors(t *testing.T) {
	f, m := newFake()
	m.noRouter = true
	m.apiAccept[^uint32(0)] = 1
	readErr := errors.New("entry re-read failed")
	restoreErr := errors.New("local restore failed")
	onDelete(f, m, ^uint32(0), func() {
		f.Fail("ip_mroute_dump", readErr)
		f.Fail("ip_mroute_add_del", restoreErr)
	})
	d := NewItfPair(f, "w5")
	_, err := d.Create(context.Background(), w5DefaultPair.Proto())
	if !errors.Is(err, readErr) || !errors.Is(err, restoreErr) {
		t.Fatalf("create error %v must preserve read and restore failures", err)
	}
	if _, ok := m.pairs[7]; ok {
		t.Fatal("pair created after failed recovery")
	}
	if got, want := mrouteOrder(t, f.Calls()), []string{"mroute add=false table 0 local", "mroute add=true table 0 local"}; !slices.Equal(got, want) {
		t.Fatalf("calls %q, want %q", got, want)
	}
}

// Review finding 2, the same gap in setAPIAccept: a netns pair's Delete whose re-read after the
// local delete fails gives the local path back and fails (the pair stays); the retried Delete
// finishes and leaves no API path.
func TestNetnsPairDeleteRestoresLocalOnReReadError(t *testing.T) {
	f, m := newFake()
	ctx := context.Background()
	d := NewItfPair(f, "w5")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: "ns-w5"}.Proto()
	meta, err := d.Create(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	onDelete(f, m, ^uint32(0), func() { f.Fail("ip_mroute_dump", boom) })
	if err := d.Delete(ctx, v, meta); !errors.Is(err, boom) {
		t.Fatalf("delete: %v, want the read error", err)
	}
	if m.apiAccept[7] != 0 || m.apiAccept[^uint32(0)] != 1 {
		t.Fatalf("after the failed check: %v, want the Accept gone and the local path given back", m.apiAccept)
	}
	if _, ok := m.pairs[7]; !ok {
		t.Fatal("pair deleted although Delete failed")
	}
	f.On("ip_mroute_dump", m.dump224)
	if err := d.Delete(ctx, v, meta); err != nil {
		t.Fatalf("retried delete: %v", err)
	}
	if _, ok := m.pairs[7]; ok || len(m.apiAccept) != 0 {
		t.Fatalf("after the retried delete: pair %v, API paths %v", ok, m.apiAccept)
	}
}

// Review NIT 4: a delete is logged at Info only when the re-read entry changed; when VPP ignored it
// (the path was linux-cp's) it is logged at Debug.
func TestLeftoverDeleteLogLevel(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(m *model)
		want  []string // "LEVEL fragment" of each "lcp:" log line, in order
	}{
		{"leftover removed", func(m *model) {
			m.noRouter = true
			m.apiAccept[^uint32(0)] = 1
		}, []string{"INFO deleted the API local path"}},
		{"lone local path of linux-cp", func(*model) {}, []string{"DEBUG sent the API local delete"}},
		{"moved pair's API Accept", func(m *model) {
			m.noRouter = true
			m.pairs[8] = w6NetnsPair()
			m.table[8] = 6
			m.apiAccept[8], m.apiAccept[^uint32(0)] = 1, 1
		}, []string{"INFO deleted the API (*,224.0.0.0/24) Accept", "INFO deleted the API local path"}},
		{"stale linux-cp Accept on the moved pair's index", func(m *model) {
			m.pairs[8] = w6NetnsPair()
			m.table[8] = 6
			m.lowAccept[8] = true
		}, []string{"DEBUG sent the API (*,224.0.0.0/24) Accept delete"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			old := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { slog.SetDefault(old) })
			f, m := newFake()
			tc.setup(m)
			if _, err := NewItfPair(f, "w5").Create(context.Background(), w5DefaultPair.Proto()); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, line := range strings.Split(buf.String(), "\n") {
				if strings.Contains(line, `msg="lcp: `) {
					got = append(got, line)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("lcp log lines %q, want %q", got, tc.want)
			}
			for i, w := range tc.want {
				level, frag, _ := strings.Cut(w, " ")
				if !strings.Contains(got[i], "level="+level+" ") || !strings.Contains(got[i], frag) {
					t.Fatalf("log line %d %q, want %s %q", i, got[i], level, frag)
				}
			}
		})
	}
}
