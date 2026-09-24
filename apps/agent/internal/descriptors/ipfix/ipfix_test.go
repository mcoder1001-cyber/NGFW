package ipfix

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"go.fd.io/govpp/api"

	classifyapi "ngfw/agent/binapi/classify"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/ipfix_export"
	"ngfw/agent/internal/descriptors/classify"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/bootid"
)

// newFake models the exporter pool (index 0 = default exporter) the way flow_api.c does:
// create_delete looks exporters up by collector address.
func newFake() (*dfkittest.FakeVPP, *[]ipfix_export.IpfixAllExporterDetails) {
	f := dfkittest.NewFake()
	pool := []ipfix_export.IpfixAllExporterDetails{{PathMtu: 512, TemplateInterval: 20, VrfID: NoVRF,
		CollectorAddress: ip_types.NewAddress(netip.IPv4Unspecified().AsSlice()), SrcAddress: ip_types.NewAddress(netip.IPv4Unspecified().AsSlice())}}
	lookup := func(a ip_types.Address) int {
		for i := 1; i < len(pool); i++ {
			if pool[i].CollectorAddress == a {
				return i
			}
		}
		return -1
	}
	f.On("ipfix_exporter_create_delete", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*ipfix_export.IpfixExporterCreateDelete)
		i := lookup(r.CollectorAddress)
		if !r.IsCreate {
			if i < 0 {
				return []api.Message{&ipfix_export.IpfixExporterCreateDeleteReply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil
			}
			pool = append(pool[:i], pool[i+1:]...)
			return []api.Message{&ipfix_export.IpfixExporterCreateDeleteReply{}}, nil
		}
		e := ipfix_export.IpfixAllExporterDetails{CollectorAddress: r.CollectorAddress, CollectorPort: r.CollectorPort,
			SrcAddress: r.SrcAddress, VrfID: r.VrfID, PathMtu: r.PathMtu, TemplateInterval: r.TemplateInterval, UDPChecksum: r.UDPChecksum}
		if i < 0 {
			pool = append(pool, e)
		} else {
			pool[i] = e
		}
		return []api.Message{&ipfix_export.IpfixExporterCreateDeleteReply{StatIndex: 3}}, nil
	})
	f.On("ipfix_all_exporter_get", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for i := range pool {
			d := pool[i]
			out = append(out, &d)
		}
		return append(out, &ipfix_export.IpfixAllExporterGetReply{Cursor: ^uint32(0)}), nil
	})
	f.On("set_ipfix_exporter", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*ipfix_export.SetIpfixExporter)
		pool[0] = ipfix_export.IpfixAllExporterDetails{CollectorAddress: r.CollectorAddress, CollectorPort: r.CollectorPort,
			SrcAddress: r.SrcAddress, VrfID: r.VrfID, PathMtu: r.PathMtu, TemplateInterval: r.TemplateInterval, UDPChecksum: r.UDPChecksum}
		return []api.Message{&ipfix_export.SetIpfixExporterReply{}}, nil
	})
	f.On("ipfix_exporter_dump", func(api.Message) ([]api.Message, error) {
		e := pool[0]
		return []api.Message{&ipfix_export.IpfixExporterDetails{CollectorAddress: e.CollectorAddress, CollectorPort: e.CollectorPort,
			SrcAddress: e.SrcAddress, VrfID: e.VrfID, PathMtu: e.PathMtu, TemplateInterval: e.TemplateInterval, UDPChecksum: e.UDPChecksum}}, nil
	})
	f.On("set_ipfix_classify_stream", dfkittest.Retval(&ipfix_export.SetIpfixClassifyStreamReply{}))
	tables := map[uint32]bool{}
	f.On("ipfix_classify_table_add_del", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*ipfix_export.IpfixClassifyTableAddDel)
		rv := int32(0)
		switch {
		case r.IsAdd && tables[r.TableID]:
			rv = int32(api.VALUE_EXIST)
		case !r.IsAdd && !tables[r.TableID]:
			rv = int32(api.NO_SUCH_ENTRY)
		default:
			tables[r.TableID] = r.IsAdd
		}
		return []api.Message{&ipfix_export.IpfixClassifyTableAddDelReply{Retval: rv}}, nil
	})
	return f, &pool
}

var slotPool = netip.MustParsePrefix("10.5.0.0/16")

func TestExporterLifecycle(t *testing.T) {
	f, pool := newFake()
	d := NewExporter(f, WithCollectorScope(slotPool.Contains))
	ctx := context.Background()
	v := Exporter{Collector: "10.5.0.9", CollectorPort: 4739, Src: "10.5.0.10", VRF: 5001, PathMTU: 1400, TemplateInterval: 20}.Proto()
	if d.KeyOf(v) != "ipfix.exporter/10.5.0.9" {
		t.Fatal(d.KeyOf(v))
	}
	if deps := d.Dependencies(v); len(deps) != 1 || deps[0].Key != "vrf/5001" || !deps[0].Optional {
		t.Fatalf("deps %+v", deps)
	}
	meta, err := d.Create(ctx, v)
	if err != nil || meta != (ExporterMeta{StatIndex: 3}) {
		t.Fatalf("create %v %v", meta, err)
	}
	// another slot's exporter and the default exporter are not ours
	*pool = append(*pool, ipfix_export.IpfixAllExporterDetails{CollectorAddress: ip_types.NewAddress([]byte{10, 6, 0, 9}), PathMtu: 512})
	dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, v))
	if n := len(dfkittest.MustRetrieve(t, d)); n != 1 {
		t.Fatalf("retrieved %d", n)
	}
	dfkittest.AssertEmptyPlan(t, d, dfkittest.KV(d, v))
	v2 := Exporter{Collector: "10.5.0.9", CollectorPort: 4740, Src: "10.5.0.10", VRF: 0, PathMTU: 1200, TemplateInterval: 5, UDPChecksum: true}.Proto()
	if _, err := d.Update(ctx, v, v2, meta); err != nil {
		t.Fatal(err)
	}
	dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, v2))
	for range 2 {
		if err := d.Delete(ctx, v2, meta); err != nil {
			t.Fatal(err)
		}
	}
	dfkittest.AssertAbsent(t, d, d.KeyOf(v2))
	for _, bad := range []Exporter{
		{Collector: "10.6.0.9", CollectorPort: 4739, Src: "10.6.0.1", PathMTU: 1400, TemplateInterval: 20}, // out of scope
		{Collector: "10.5.0.9", CollectorPort: 0, Src: "10.5.0.1", PathMTU: 1400, TemplateInterval: 20},
		{Collector: "10.5.0.9", CollectorPort: 4739, Src: "10.5.0.1", PathMTU: 1500, TemplateInterval: 20},
		{Collector: "10.5.0.9", CollectorPort: 4739, Src: "fd00::1", PathMTU: 1400, TemplateInterval: 20},
		{Collector: "10.5.0.9", CollectorPort: 4739, Src: "10.5.0.1", PathMTU: 1400},
	} {
		if _, err := d.Create(ctx, bad.Proto()); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

func TestDefaultExporter(t *testing.T) {
	f, pool := newFake()
	d := NewDefaultExporter(f, WithGlobals(dfkit.GlobalsOwner(true)))
	ctx := context.Background()
	if kvs := dfkittest.MustRetrieve(t, d); len(kvs) != 0 { // unset → not reported
		t.Fatalf("unset exporter reported: %v", kvs)
	}
	v := Exporter{Collector: "10.5.0.9", CollectorPort: 4739, Src: "10.5.0.10", VRF: NoVRF, PathMTU: 1400, TemplateInterval: 20}.Proto()
	if d.KeyOf(v) != KeyDefaultExporter || d.Dependencies(v) != nil {
		t.Fatal("key/deps")
	}
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	dfkittest.AssertRetrieved(t, d, dfkittest.KV(d, v))
	dfkittest.AssertEmptyPlan(t, d, dfkittest.KV(d, v))
	// the additional-exporter descriptor never reports exporter 0
	if kvs := dfkittest.MustRetrieve(t, NewExporter(f)); len(kvs) != 0 {
		t.Fatalf("exporter 0 leaked into ipfix.exporter: %v", kvs)
	}
	// D-071: a non-owner only requires exporter 0 — satisfied when VPP has exactly it, never set
	other := NewDefaultExporter(f)
	if _, err := other.Create(ctx, v); err != nil {
		t.Fatalf("non-owner requirement met: %v", err)
	}
	v3 := Exporter{Collector: "10.5.0.99", CollectorPort: 4739, Src: "10.5.0.10", VRF: NoVRF, PathMTU: 1400, TemplateInterval: 20}.Proto()
	sets := len(f.CallsNamed("set_ipfix_exporter"))
	if _, err := other.Create(ctx, v3); !errors.Is(err, dfkit.ErrNotGlobalsOwner) {
		t.Fatalf("non-owner requirement not met: %v", err)
	}
	if err := other.Delete(ctx, v, nil); err != nil || len(f.CallsNamed("set_ipfix_exporter")) != sets {
		t.Fatalf("non-owner must never set or reset: %v", err)
	}
	if _, err := other.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
		t.Fatalf("non-owner retrieve: %v", err)
	}
	if err := d.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
	if !dfkit.FromAPIAddress((*pool)[0].CollectorAddress).IsUnspecified() {
		t.Fatal("delete did not unset the collector")
	}
	dfkittest.AssertAbsent(t, d, KeyDefaultExporter)
	v6 := Exporter{Collector: "fd00::9", CollectorPort: 4739, Src: "fd00::10", PathMTU: 1400, TemplateInterval: 20}.Proto()
	if _, err := d.Create(ctx, v6); !errors.Is(err, dfkit.ErrSpec) {
		t.Fatalf("ipv6 default exporter: %v", err)
	}
}

// classifyFake adds DF-2's classify table (index 3 = name "w5-t") to the fake.
func classifyFake(t *testing.T, f *dfkittest.FakeVPP) classify.Store {
	t.Helper()
	mask := make([]byte, 16)
	mask[12] = 0xff
	f.Reply("classify_table_ids", &classifyapi.ClassifyTableIdsReply{Ids: []uint32{3}, Count: 1})
	f.Reply("classify_table_info", &classifyapi.ClassifyTableInfoReply{TableID: 3, MatchNVectors: 1, Mask: mask, MaskLength: 16})
	st := classify.NewMemStore()
	t.Cleanup(bootid.SetProcRoot(t.TempDir()))          // the fake PID is no real process: empty /proc
	cur, err := bootid.Current(context.Background(), f) // the running (fake) VPP
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Reset(cur); err != nil {
		t.Fatal(err)
	}
	if err := st.Put(classify.TableRecord{Name: "w5-t", Index: 3, MatchNVectors: 1, Mask: mask}); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestClassifyWriteOnly(t *testing.T) {
	f, _ := newFake()
	ctx := context.Background()
	cs := NewClassifyStream(f, WithGlobals(dfkit.GlobalsOwner(true)))
	if _, err := NewClassifyStream(f).Create(ctx, ClassifyStream{DomainID: 5, SrcPort: 4744}.Proto()); !errors.Is(err, dfkit.ErrNotGlobalsOwner) {
		t.Fatalf("non-owner stream: %v", err)
	}
	st := classifyFake(t, f)
	ct := NewClassifyTable(f, st)
	sv := ClassifyStream{DomainID: 5, SrcPort: 4744}.Proto()
	tv := ClassifyTable{Table: "w5-t", IPVersion: "ip6", Protocol: 6}.Proto()
	if k := ct.KeyOf(tv); k != "ipfix.classify-table/w5-t" {
		t.Fatal(k)
	}
	if deps := ct.Dependencies(tv); len(deps) != 2 || deps[0].Key != classify.TableKey("w5-t") || deps[0].Optional || deps[1].Key != KeyClassifyStream || !deps[1].Optional {
		t.Fatalf("deps %+v", deps)
	}
	if deps := cs.Dependencies(sv); len(deps) != 1 || deps[0].Key != KeyDefaultExporter || !deps[0].Optional {
		t.Fatalf("stream deps %+v", deps)
	}
	for range 2 {
		if _, err := cs.Create(ctx, sv); err != nil {
			t.Fatal(err)
		}
		if _, err := ct.Create(ctx, tv); err != nil { // second: VALUE_EXIST = success
			t.Fatal(err)
		}
	}
	req := f.CallsNamed("ipfix_classify_table_add_del")[0].(*ipfix_export.IpfixClassifyTableAddDel)
	if req.TableID != 3 || req.IPVersion != ip_types.ADDRESS_IP6 || req.TransportProtocol != 6 || !req.IsAdd {
		t.Fatalf("request %+v (the index must come from DF-2's live record)", req)
	}
	// H2: a name that is not a live table of ours never becomes an index
	if _, err := ct.Create(ctx, ClassifyTable{Table: "w6-t", IPVersion: "ip4"}.Proto()); !errors.Is(err, ErrNoClassifyTable) {
		t.Fatalf("unknown table: %v", err)
	}
	for _, d := range []scheduler.Descriptor{cs, ct} {
		if kvs, err := d.Retrieve(ctx); kvs != nil || !errors.Is(err, dfkit.ErrRetrieveUnsupported) || !errors.Is(err, ErrClassifyDumpBroken) {
			t.Fatalf("%s retrieve %v %v", d.Name(), kvs, err)
		}
	}
	if len(f.CallsNamed("ipfix_classify_stream_dump")) != 0 || len(f.CallsNamed("ipfix_classify_table_dump")) != 0 {
		t.Fatal("the broken dumps must not be sent")
	}
	if _, err := ct.Update(ctx, tv, tv, nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	for range 2 {
		if err := ct.Delete(ctx, tv, nil); err != nil {
			t.Fatal(err)
		}
	}
	// the table is gone from VPP (DF-2 deleted it / VPP restarted): Delete never uses a stale index
	f.Reply("classify_table_ids", &classifyapi.ClassifyTableIdsReply{})
	dels := len(f.CallsNamed("ipfix_classify_table_add_del"))
	if err := ct.Delete(ctx, tv, nil); err != nil || len(f.CallsNamed("ipfix_classify_table_add_del")) != dels {
		t.Fatalf("delete of a gone table: %v", err)
	}
	if err := cs.Delete(ctx, sv, nil); err != nil {
		t.Fatal(err)
	}
	last := f.CallsNamed("set_ipfix_classify_stream")
	if r := last[len(last)-1].(*ipfix_export.SetIpfixClassifyStream); r.SrcPort != 0 || r.DomainID != 0 {
		t.Fatalf("delete must unset the stream: %+v", r)
	}
	if _, err := cs.Create(ctx, ClassifyStream{DomainID: 1}.Proto()); !errors.Is(err, dfkit.ErrSpec) {
		t.Fatal(err)
	}
	if _, err := ct.Create(ctx, ClassifyTable{Table: "w5-t", IPVersion: "ipx"}.Proto()); !errors.Is(err, dfkit.ErrSpec) {
		t.Fatal(err)
	}
	// the decoder that replaces the write-only Retrieve once VPP is fixed
	kvs := ct.DecodeClassifyTables([]*ipfix_export.IpfixClassifyTableDetails{
		{TableID: 3, IPVersion: ip_types.ADDRESS_IP6, TransportProtocol: 6}, {TableID: 42},
	}, []classify.TableRecord{{Name: "w5-t", Index: 3}})
	if len(kvs) != 1 || kvs[0].Key != "ipfix.classify-table/w5-t" || !protoEqual(kvs[0], dfkittest.KV(ct, tv)) {
		t.Fatalf("decoded %v", kvs)
	}
}

func protoEqual(a, b scheduler.KV) bool {
	return dfkittest.DiffPlan([]scheduler.KV{a}, []scheduler.KV{b}).Empty()
}

func TestRegister(t *testing.T) {
	r := scheduler.NewRegistry()
	f := dfkittest.NewFake()
	RegisterGlobals(r, f)
	Register(r, f, classify.NewMemStore())
	if r.Len() != 4 {
		t.Fatalf("registered %v", r.Names())
	}
}
