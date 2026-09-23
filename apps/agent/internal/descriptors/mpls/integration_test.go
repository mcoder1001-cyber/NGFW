package mpls

import (
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/mpls"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Host test: MPLS tables <base>+90/+91 named "<prefix>:<id>", routes and a tunnel via this
// slot's loopback loop<slot>90 (10.<slot>.90.1/24). MPLS table 0 is VPP-global (MPLS on an
// interface and label bindings need it and would create/lock it): those subtests run only when
// table 0 already exists, or for the globals owner (VRX_DF7_GLOBALS=1, D-071).
func TestMPLSOnHost(t *testing.T) {
	h := df7test.StartHost(t)
	td, id, rd, bd, nd := NewTable(h.C, h.Owner), NewInterface(h.C, h.Owner), NewRoute(h.C, h.Owner), NewIPBind(h.C, h.Owner), NewTunnel(h.C, h.Owner)
	ifA, idxA := h.Loopback(90, true, true)
	h.Address(idxA, h.Addr(90, 1)+"/24")
	h.CleanupOwned(td)
	h.CleanupOwned(rd) // registered later → routes are removed before their tables (V15)
	h.CleanupOwned(nd)

	t1, t2 := h.TableB+90, h.TableB+91
	tables := []scheduler.KV{df7test.Desired(td, df7.Encode(Table{ID: t1})), df7test.Desired(td, df7.Encode(Table{ID: t2}))}
	ct := h.Apply(td, tables...)
	h.ExpectRetrieved(td, tables...)

	path := func(nh string, labels ...uint32) df7.Path {
		p := df7.Path{Interface: ifA, NextHop: nh}
		for _, l := range labels {
			p.Labels = append(p.Labels, df7.Label{Label: l})
		}
		n, err := df7.NormalizePaths([]df7.Path{p})
		h.Must("normalize", err)
		return n[0]
	}
	routes := []scheduler.KV{
		df7test.Desired(rd, df7.Encode(Route{Table: t1, Label: t1, EOS: true, EOSProto: PayloadIP4, Paths: []df7.Path{path(h.Addr(90, 2), 100, 200)}})),
		df7test.Desired(rd, df7.Encode(Route{Table: t1, Label: t1 + 1, Paths: []df7.Path{path(h.Addr(90, 3), 300)}})),
		df7test.Desired(rd, df7.Encode(Route{Table: t2, Label: t2, EOS: true, EOSProto: PayloadIP6, Paths: []df7.Path{{Type: df7.PathDrop, Proto: df7.ProtoIP6, Weight: 1}}})),
	}
	cr := h.Apply(rd, routes...)
	h.ExpectRetrieved(rd, routes...)

	t.Run("route paths update in place", func(*testing.T) {
		n := df7.Encode(Route{Table: t1, Label: t1, EOS: true, EOSProto: PayloadIP4, Paths: []df7.Path{path(h.Addr(90, 2), 100, 200), path(h.Addr(90, 4), 101)}})
		_, err := rd.Update(h.Ctx, routes[0].Value, n, nil)
		h.Must("update", err)
		routes[0].Value, cr[0].Value = n, n
		h.ExpectRetrieved(rd, routes...)
	})

	tunnels := []scheduler.KV{df7test.Desired(nd, df7.Encode(Tunnel{Name: h.Owner + "-mt1", Paths: []df7.Path{path(h.Addr(90, 2), 500, 600)}}))}
	cn := h.Apply(nd, tunnels...)
	h.ExpectRetrieved(nd, tunnels...)
	if idx, ok := h.IfIndex("mpls-tunnel" + itoa(cn[0].Meta.(TunnelMeta).TunnelIndex)); ok {
		t.Logf("tunnel interface mpls-tunnel%d sw_if_index %d, tagged %s:%s → logical name %s", cn[0].Meta.(TunnelMeta).TunnelIndex, idx, h.Owner, h.Owner+"-mt1", h.Owner+"-mt1")
	}

	t.Run("interface + ip bind (need MPLS table 0)", func(t *testing.T) {
		if !table0(t, h) {
			// without table 0 VPP refuses the enable (and changes nothing): show it
			_, err := id.Create(h.Ctx, df7.Encode(Interface{Interface: ifA}))
			t.Logf("mpls-interface Create without MPLS table 0: %v", err)
			if !df7.IsVPPError(err, api.NO_SUCH_FIB) {
				t.Fatalf("want NO_SUCH_FIB, got %v", err)
			}
			h.Must("release claim", id.Delete(h.Ctx, df7.Encode(Interface{Interface: ifA}), nil))
			df7test.GlobalsOptIn(t, "MPLS table 0 (created/locked by mpls enable and label bindings)")
		}
		iv := df7test.Desired(id, df7.Encode(Interface{Interface: ifA}))
		ci := h.Apply(id, iv)
		h.ExpectRetrieved(id, iv)
		h.DeleteAll(id, ci)
		h.ExpectNone(id)
		h.IPTable(h.TableB + 92)
		b := df7.Encode(IPBind{MPLSTable: t1, Label: t1 + 50, VRF: h.TableB + 92, Prefix: h.Addr(92, 0) + "/24"})
		cb := h.Apply(bd, df7test.Desired(bd, b))
		h.Apply(bd, df7test.Desired(bd, b)) // idempotent
		h.DeleteAll(bd, cb)
	})
	if _, err := bd.Retrieve(h.Ctx); !errors.Is(err, df7.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}

	h.Hold("mpls tables/routes/tunnel")

	t.Run("restart simulation", func(t *testing.T) {
		h.ExpectRetrieved(NewTable(df7test.Connect(t), h.Owner), tables...)
		h.RestartSimulation(func(c vpp.Client) scheduler.Descriptor { return NewRoute(c, h.Owner) }, routes...)
		h.RestartSimulation(func(c vpp.Client) scheduler.Descriptor { return NewTunnel(c, h.Owner) }, tunnels...)
	})

	h.DeleteAll(nd, cn)
	h.ExpectNone(nd)
	h.DeleteAll(rd, cr)
	h.ExpectNone(rd)
	t.Run("restart simulation (tables, after their routes are gone)", func(*testing.T) {
		h.RestartSimulation(func(c vpp.Client) scheduler.Descriptor { return NewTable(c, h.Owner) }, tables...)
	})
	h.DeleteAll(td, ct)
	h.ExpectNone(td)
}

// table0 reports whether MPLS table 0 exists.
func table0(t *testing.T, h *df7test.Host) bool {
	t.Helper()
	stream, err := mpls.NewServiceClient(h.C).MplsTableDump(h.Ctx, &mpls.MplsTableDump{})
	h.Must("mpls_table_dump", err)
	dets, err := df7.Collect(stream.Recv)
	h.Must("mpls_table_dump", err)
	for _, d := range dets {
		if d.MtTable.MtTableID == 0 {
			return true
		}
	}
	return false
}

func itoa(v uint32) string { return u32(v) }
