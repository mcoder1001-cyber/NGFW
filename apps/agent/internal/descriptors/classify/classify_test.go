package classify

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	classifyapi "ngfw/agent/binapi/classify"
	interfaces "ngfw/agent/binapi/interface"
	isr "ngfw/agent/binapi/ip_session_redirect"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

type table struct {
	info     classifyapi.ClassifyTableInfoReply
	sessions map[string]classifyapi.ClassifySessionDetails // by hex(full match)
}

type binding struct{ ip4, ip6, l2 uint32 }

type fakeVPP struct {
	*fake.Client
	next     uint32
	tables   map[uint32]*table
	inputACL map[uint32]binding
	// redirects: full matches of ip_session_redirect sessions per table index.
	redirects map[uint32][][]byte
}

func newFakeVPP() *fakeVPP {
	v := &fakeVPP{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})), tables: map[uint32]*table{}, inputACL: map[uint32]binding{}}
	v.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 5, InterfaceName: "loop300", Tag: "w3:loop300"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 7, InterfaceName: "loop200", Tag: "w2:loop200"},
	)
	v.On("classify_add_del_table", func(req api.Message) ([]api.Message, error) {
		r := req.(*classifyapi.ClassifyAddDelTable)
		if !r.IsAdd {
			if _, ok := v.tables[r.TableIndex]; !ok {
				return []api.Message{&classifyapi.ClassifyAddDelTableReply{Retval: -6}}, nil
			}
			delete(v.tables, r.TableIndex)
			return []api.Message{&classifyapi.ClassifyAddDelTableReply{}}, nil
		}
		if r.NextTableIndex != NoIndex {
			if _, ok := v.tables[r.NextTableIndex]; !ok {
				return []api.Message{&classifyapi.ClassifyAddDelTableReply{Retval: -1}}, nil
			}
		}
		idx := v.next
		v.next++
		v.tables[idx] = &table{info: classifyapi.ClassifyTableInfoReply{TableID: idx, Nbuckets: r.Nbuckets, MatchNVectors: r.MatchNVectors, SkipNVectors: r.SkipNVectors,
			NextTableIndex: r.NextTableIndex, MissNextIndex: r.MissNextIndex, MaskLength: uint32(len(r.Mask)), Mask: append([]byte(nil), r.Mask...)}, sessions: map[string]classifyapi.ClassifySessionDetails{}} //nolint:gosec // test sizes
		return []api.Message{&classifyapi.ClassifyAddDelTableReply{NewTableIndex: idx, SkipNVectors: r.SkipNVectors, MatchNVectors: r.MatchNVectors}}, nil
	})
	v.On("classify_table_ids", func(api.Message) ([]api.Message, error) {
		rep := &classifyapi.ClassifyTableIdsReply{}
		for idx := range v.tables {
			rep.Ids = append(rep.Ids, idx)
		}
		return []api.Message{rep}, nil
	})
	v.On("classify_table_info", func(req api.Message) ([]api.Message, error) {
		t, ok := v.tables[req.(*classifyapi.ClassifyTableInfo).TableID]
		if !ok {
			return []api.Message{&classifyapi.ClassifyTableInfoReply{Retval: -6}}, nil
		}
		info := t.info
		return []api.Message{&info}, nil
	})
	v.On("classify_add_del_session", func(req api.Message) ([]api.Message, error) {
		r := req.(*classifyapi.ClassifyAddDelSession)
		t, ok := v.tables[r.TableIndex]
		if !ok {
			return []api.Message{&classifyapi.ClassifyAddDelSessionReply{Retval: -6}}, nil
		}
		if uint32(len(r.Match)) != (t.info.SkipNVectors+t.info.MatchNVectors)*VectorSize { //nolint:gosec // test sizes
			return []api.Message{&classifyapi.ClassifyAddDelSessionReply{Retval: -1}}, nil
		}
		k := hex.EncodeToString(r.Match)
		if r.IsAdd {
			t.sessions[k] = classifyapi.ClassifySessionDetails{TableID: r.TableIndex, HitNextIndex: r.HitNextIndex, Advance: r.Advance, OpaqueIndex: r.OpaqueIndex,
				MatchLength: t.info.MatchNVectors * VectorSize, Match: append([]byte(nil), r.Match[t.info.SkipNVectors*VectorSize:]...)}
		} else {
			delete(t.sessions, k)
		}
		return []api.Message{&classifyapi.ClassifyAddDelSessionReply{}}, nil
	})
	v.On("classify_session_dump", func(req api.Message) ([]api.Message, error) {
		t, ok := v.tables[req.(*classifyapi.ClassifySessionDump).TableID]
		if !ok {
			return nil, nil
		}
		var out []api.Message
		for _, s := range t.sessions {
			d := s
			out = append(out, &d)
		}
		return out, nil
	})
	v.On("ip_session_redirect_dump", func(req api.Message) ([]api.Message, error) {
		idx := req.(*isr.IPSessionRedirectDump).TableIndex
		var out []api.Message
		for _, m := range v.redirects[idx] {
			out = append(out, &isr.IPSessionRedirectDetails{TableIndex: idx, MatchLength: uint32(len(m)), Match: m})
		}
		return out, nil
	})
	v.Reply("classify_set_interface_ip_table", &classifyapi.ClassifySetInterfaceIPTableReply{})
	v.Reply("classify_set_interface_l2_tables", &classifyapi.ClassifySetInterfaceL2TablesReply{})
	v.Reply("output_acl_set_interface", &classifyapi.OutputACLSetInterfaceReply{})
	v.On("input_acl_set_interface", func(req api.Message) ([]api.Message, error) {
		r := req.(*classifyapi.InputACLSetInterface)
		if r.IsAdd {
			v.inputACL[uint32(r.SwIfIndex)] = binding{r.IP4TableIndex, r.IP6TableIndex, r.L2TableIndex}
		} else {
			delete(v.inputACL, uint32(r.SwIfIndex))
		}
		return []api.Message{&classifyapi.InputACLSetInterfaceReply{}}, nil
	})
	v.On("classify_table_by_interface", func(req api.Message) ([]api.Message, error) {
		idx := req.(*classifyapi.ClassifyTableByInterface).SwIfIndex
		b, ok := v.inputACL[uint32(idx)]
		if !ok {
			b = binding{NoIndex, NoIndex, NoIndex}
		}
		return []api.Message{&classifyapi.ClassifyTableByInterfaceReply{SwIfIndex: idx, IP4TableID: b.ip4, IP6TableID: b.ip6, L2TableID: b.l2}}, nil
	})
	return v
}

func TestStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "classify.json")
	fs, err := OpenFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []Store{NewMemStore(), fs} {
		if err := s.Put(TableRecord{Name: "b", Index: 2}); err != nil {
			t.Fatal(err)
		}
		if err := s.Put(TableRecord{Name: "a", Index: 1, Sessions: map[string]SessionRecord{"0a": {Action: 3, Metadata: 9}}}); err != nil {
			t.Fatal(err)
		}
		if r, ok := s.Get("a"); !ok || r.Index != 1 || r.Sessions["0a"].Metadata != 9 {
			t.Fatalf("Get = %+v %v", r, ok)
		}
		if all := s.All(); len(all) != 2 || all[0].Name != "a" || all[1].Name != "b" {
			t.Fatalf("All = %+v", all)
		}
		if err := s.Delete("b"); err != nil {
			t.Fatal(err)
		}
		if _, ok := s.Get("b"); ok {
			t.Fatal("deleted record still present")
		}
	}
	// The file store survives a reopen.
	fs2, err := OpenFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := fs2.Get("a"); !ok || r.Index != 1 || r.Sessions["0a"].Action != 3 {
		t.Fatalf("reopened = %+v %v", r, ok)
	}
	if _, err := OpenFileStore(filepath.Join(t.TempDir(), "missing.json")); err != nil {
		t.Fatalf("missing file must be an empty store: %v", err)
	}
}

func TestTableLifecycle(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	store := NewMemStore()
	d := NewTable(v, store)
	if !scheduler.ValidName(d.Name()) {
		t.Fatal(d.Name())
	}
	// Someone else's table (not in our store) exists in VPP.
	if _, err := classifyapi.NewServiceClient(v).ClassifyAddDelTable(ctx, &classifyapi.ClassifyAddDelTable{IsAdd: true, NextTableIndex: NoIndex, MissNextIndex: NoIndex, MatchNVectors: 1, Mask: make([]byte, 16)}); err != nil {
		t.Fatal(err)
	}
	mask := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 255, 255}
	desired := &Table{Name: "w3-t1", Nbuckets: 3, SkipNVectors: 1, MatchNVectors: 1, Mask: mask[:16], MissNextIndex: NoIndex}
	norm := NormalizeTable(desired)
	if norm.Nbuckets != 4 || norm.MemorySize != DefaultMemorySize || len(norm.Mask) != 16 {
		t.Fatalf("Normalize = %+v", norm)
	}
	if k := d.KeyOf(desired); k != "classify.table/w3-t1" || d.Dependencies(desired) != nil {
		t.Fatalf("KeyOf/Dependencies")
	}
	meta, err := d.Create(ctx, desired)
	if err != nil || meta != (TableMeta{Index: 1}) {
		t.Fatalf("Create: %v %+v", err, meta)
	}
	req := v.CallsNamed("classify_add_del_table")[1].(*classifyapi.ClassifyAddDelTable)
	if !req.IsAdd || req.TableIndex != NoIndex || req.Nbuckets != 4 || req.MemorySize != DefaultMemorySize || req.SkipNVectors != 1 || req.MatchNVectors != 1 ||
		req.NextTableIndex != NoIndex || req.MissNextIndex != NoIndex || req.MaskLen != 16 || !bytes.Equal(req.Mask, mask) {
		t.Fatalf("request = %+v", req)
	}
	rec, ok := store.Get("w3-t1")
	if !ok || rec.Index != 1 || rec.SkipNVectors != 1 || rec.MatchNVectors != 1 {
		t.Fatalf("store = %+v %v", rec, ok)
	}
	// A chained table depends on and resolves its next table.
	chained := &Table{Name: "w3-t2", MatchNVectors: 2, Mask: []byte{1}, NextTable: "w3-t1", MissNextIndex: 5, CurrentDataFlag: true, CurrentDataOffset: -4, MemorySize: 4096}
	if deps := d.Dependencies(chained); len(deps) != 1 || deps[0].Key != "classify.table/w3-t1" {
		t.Fatalf("Dependencies = %+v", deps)
	}
	if _, err := d.Create(ctx, chained); err != nil {
		t.Fatal(err)
	}
	req = v.CallsNamed("classify_add_del_table")[2].(*classifyapi.ClassifyAddDelTable)
	if req.NextTableIndex != 1 || req.CurrentDataFlag != 1 || req.CurrentDataOffset != -4 || req.MaskLen != 32 {
		t.Fatalf("chained request = %+v", req)
	}
	actual, err := d.Retrieve(ctx)
	if err != nil || len(actual) != 2 {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	for _, want := range []*Table{norm, NormalizeTable(chained)} {
		var got *scheduler.KV
		for i := range actual {
			if actual[i].Key == d.KeyOf(want) {
				got = &actual[i]
			}
		}
		if got == nil || !proto.Equal(got.Value, want) {
			t.Fatalf("Retrieve[%s] = %+v\nwant %+v", d.KeyOf(want), got, want)
		}
	}
	if _, err := d.Update(ctx, desired, norm, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if _, err := d.Create(ctx, desired); err == nil {
		t.Fatal("duplicate name accepted")
	}
	if _, err := d.Create(ctx, &Table{Name: "w3-t3", NextTable: "nope"}); !errors.Is(err, ErrNoSuchTable) {
		t.Fatalf("unknown next table: %v", err)
	}
	// Delete uses Meta, or the store when Meta is missing; both clear the store.
	if err := d.Delete(ctx, chained, nil); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); len(actual) != 0 || len(store.All()) != 0 || len(v.tables) != 1 {
		t.Fatalf("after Delete: %+v store=%+v tables=%d", actual, store.All(), len(v.tables))
	}
	// A stale store record (table vanished from VPP) is dropped, never reported.
	_ = store.Put(TableRecord{Name: "w3-gone", Index: 42})
	if actual, err := d.Retrieve(ctx); err != nil || len(actual) != 0 || len(store.All()) != 0 {
		t.Fatalf("stale record: %+v %v store=%+v", actual, err, store.All())
	}
	if err := d.Delete(ctx, desired, "bad"); !errors.Is(err, df2.ErrBadMeta) {
		t.Fatalf("bad meta: %v", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	store := NewMemStore()
	td := NewTable(v, store)
	if _, err := td.Create(ctx, &Table{Name: "w3-t1", SkipNVectors: 1, MatchNVectors: 1, Mask: bytes.Repeat([]byte{255}, 16), MissNextIndex: NoIndex}); err != nil {
		t.Fatal(err)
	}
	d := NewSession(v, store)
	if !scheduler.ValidName(d.Name()) {
		t.Fatal(d.Name())
	}
	key := append(make([]byte, 16), 10, 3, 0, 1) // skip vector, then 4 significant bytes
	desired := &Session{Table: "w3-t1", Match: append(append([]byte(nil), key...), 0, 0), HitNextIndex: NoIndex, OpaqueIndex: 77, Advance: 8, Action: Session_SET_METADATA, Metadata: 5}
	if k := d.KeyOf(desired); k != "classify.session/w3-t1/000000000000000000000000000000000a030001" {
		t.Fatalf("KeyOf = %s", k)
	}
	if k := d.KeyOf(&Session{Table: "t", Match: nil}); k != "classify.session/t/0" {
		t.Fatalf("empty match key = %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 1 || deps[0].Key != "classify.table/w3-t1" {
		t.Fatalf("Dependencies = %+v", deps)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil || meta != (SessionMeta{TableIndex: 0}) {
		t.Fatalf("Create: %v %+v", err, meta)
	}
	req := v.CallsNamed("classify_add_del_session")[0].(*classifyapi.ClassifyAddDelSession)
	if !req.IsAdd || req.TableIndex != 0 || req.MatchLen != 32 || len(req.Match) != 32 || !bytes.Equal(req.Match[:20], key) || req.HitNextIndex != 0xFFFF ||
		req.OpaqueIndex != 77 || req.Advance != 8 || req.Action != classifyapi.CLASSIFY_API_ACTION_SET_METADATA || req.Metadata != 5 {
		t.Fatalf("request = %+v", req)
	}
	// A redirect session in the same table (created by ip-session-redirect) is not ours.
	redir := append(make([]byte, 16), 10, 3, 0, 2)
	v.tables[0].sessions[hex.EncodeToString(redir)] = classifyapi.ClassifySessionDetails{TableID: 0, HitNextIndex: 2, OpaqueIndex: NoIndex, MatchLength: 16, Match: redir[16:]}
	v.redirects = map[uint32][][]byte{0: {append(append([]byte(nil), redir...), make([]byte, 12)...)}}
	actual, err := d.Retrieve(ctx)
	norm := NormalizeSession(desired)
	if err != nil || len(actual) != 1 || actual[0].Key != d.KeyOf(desired) || !proto.Equal(actual[0].Value, norm) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v, %v\nwant %+v", actual, err, norm)
	}
	if _, err := d.Update(ctx, desired, norm, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	// Without the ip_session_redirect plugin VPP has no redirect sessions to exclude.
	v.On("ip_session_redirect_dump", func(api.Message) ([]api.Message, error) {
		return nil, &adapter.UnknownMsgError{MsgName: "ip_session_redirect_dump"}
	})
	delete(v.tables[0].sessions, hex.EncodeToString(redir))
	if actual, err = d.Retrieve(ctx); err != nil || len(actual) != 0 {
		t.Fatalf("after Delete = %+v", actual)
	}
	if rec, _ := store.Get("w3-t1"); len(rec.Sessions) != 0 {
		t.Fatalf("session extras not cleared: %+v", rec.Sessions)
	}
	if _, err := d.Create(ctx, &Session{Table: "nope", Match: key}); !errors.Is(err, ErrNoSuchTable) {
		t.Fatalf("unknown table: %v", err)
	}
	if _, err := d.Create(ctx, &Session{Table: "w3-t1", Match: append(make([]byte, 32), 1)}); err == nil {
		t.Fatal("match longer than the table geometry accepted")
	}
	if err := d.Delete(ctx, desired, SessionMeta{TableIndex: 9}); err == nil {
		t.Fatal("meta/store index mismatch accepted")
	}
}

func TestBindings(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	store := NewMemStore()
	td := NewTable(v, store)
	for _, n := range []string{"w3-ip4", "w3-ip6", "w3-l2"} {
		if _, err := td.Create(ctx, &Table{Name: n, Mask: []byte{255}, MissNextIndex: NoIndex}); err != nil {
			t.Fatal(err)
		}
	}
	// interface ip table (write-only)
	ipd := NewInterfaceIPTable(v, "w3", store)
	ipt := &InterfaceIpTable{Interface: "loop300", Ipv6: true, Table: "w3-ip6"}
	if k := ipd.KeyOf(ipt); k != "classify.interface-ip-table/loop300/ipv6" || !scheduler.ValidName(ipd.Name()) {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := ipd.Dependencies(ipt); len(deps) != 2 || deps[0].Key != "interface/loop300" || deps[1].Key != "classify.table/w3-ip6" {
		t.Fatalf("Dependencies = %+v", deps)
	}
	meta, err := ipd.Create(ctx, ipt)
	if err != nil || meta != (BindingMeta{SwIfIndex: 5}) {
		t.Fatalf("Create: %v %+v", err, meta)
	}
	req := v.CallsNamed("classify_set_interface_ip_table")[0].(*classifyapi.ClassifySetInterfaceIPTable)
	if !req.IsIPv6 || req.SwIfIndex != 5 || req.TableIndex != 1 {
		t.Fatalf("request = %+v", req)
	}
	if _, err := ipd.Update(ctx, ipt, &InterfaceIpTable{Interface: "loop300", Ipv6: true, Table: "w3-ip4"}, meta); err != nil {
		t.Fatal(err)
	}
	if req = v.CallsNamed("classify_set_interface_ip_table")[1].(*classifyapi.ClassifySetInterfaceIPTable); req.TableIndex != 0 {
		t.Fatalf("update request = %+v", req)
	}
	if _, err := ipd.Update(ctx, ipt, &InterfaceIpTable{Interface: "loop300", Table: "w3-ip4"}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("family change: %v", err)
	}
	if err := ipd.Delete(ctx, ipt, meta); err != nil {
		t.Fatal(err)
	}
	if req = v.CallsNamed("classify_set_interface_ip_table")[2].(*classifyapi.ClassifySetInterfaceIPTable); req.TableIndex != NoIndex {
		t.Fatalf("delete request = %+v", req)
	}
	if _, err := ipd.Retrieve(ctx); !errors.Is(err, df2.ErrRetrieveUnsupported) {
		t.Fatalf("Retrieve: %v", err)
	}
	if _, err := ipd.Create(ctx, &InterfaceIpTable{Interface: "loop300", Table: "nope"}); !errors.Is(err, ErrNoSuchTable) {
		t.Fatalf("unknown table: %v", err)
	}
	if _, err := ipd.Create(ctx, &InterfaceIpTable{Interface: "nope", Table: "w3-ip4"}); !errors.Is(err, df2.ErrNoSuchInterface) {
		t.Fatalf("unknown interface: %v", err)
	}

	// interface l2 tables (write-only)
	l2d := NewInterfaceL2Tables(v, "w3", store)
	l2 := &InterfaceL2Tables{Interface: "loop300", Input: true, Ip4Table: "w3-ip4", OtherTable: "w3-l2"}
	if k := l2d.KeyOf(l2); k != "classify.interface-l2-tables/loop300/input" || len(l2d.Dependencies(l2)) != 3 {
		t.Fatalf("KeyOf = %s deps=%+v", k, l2d.Dependencies(l2))
	}
	if _, err := l2d.Create(ctx, l2); err != nil {
		t.Fatal(err)
	}
	l2req := v.CallsNamed("classify_set_interface_l2_tables")[0].(*classifyapi.ClassifySetInterfaceL2Tables)
	if l2req.SwIfIndex != 5 || l2req.IP4TableIndex != 0 || l2req.IP6TableIndex != NoIndex || l2req.OtherTableIndex != 2 || !l2req.IsInput {
		t.Fatalf("request = %+v", l2req)
	}
	if err := l2d.Delete(ctx, l2, BindingMeta{SwIfIndex: 5}); err != nil {
		t.Fatal(err)
	}
	if l2req = v.CallsNamed("classify_set_interface_l2_tables")[1].(*classifyapi.ClassifySetInterfaceL2Tables); l2req.IP4TableIndex != NoIndex || l2req.OtherTableIndex != NoIndex {
		t.Fatalf("delete request = %+v", l2req)
	}
	if _, err := l2d.Create(ctx, &InterfaceL2Tables{Interface: "loop300"}); err == nil {
		t.Fatal("no tables accepted")
	}
	if _, err := l2d.Retrieve(ctx); !errors.Is(err, df2.ErrRetrieveUnsupported) {
		t.Fatalf("Retrieve: %v", err)
	}

	// input acl: the one binding VPP reads back.
	ind := NewInputACL(v, "w3", store)
	v.inputACL[7] = binding{0, NoIndex, NoIndex} // another worker's interface uses our table index by coincidence
	in := &InputAcl{Interface: "loop300", Ip4Table: "w3-ip4", L2Table: "w3-l2"}
	if k := ind.KeyOf(in); k != "classify.input-acl/loop300" || len(ind.Dependencies(in)) != 3 {
		t.Fatalf("KeyOf = %s", k)
	}
	meta, err = ind.Create(ctx, in)
	if err != nil || meta != (BindingMeta{SwIfIndex: 5}) {
		t.Fatalf("Create: %v %+v", err, meta)
	}
	inreq := v.CallsNamed("input_acl_set_interface")[0].(*classifyapi.InputACLSetInterface)
	if !inreq.IsAdd || inreq.IP4TableIndex != 0 || inreq.IP6TableIndex != NoIndex || inreq.L2TableIndex != 2 {
		t.Fatalf("request = %+v", inreq)
	}
	actual, err := ind.Retrieve(ctx)
	if err != nil || len(actual) != 1 || !proto.Equal(actual[0].Value, in) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	if _, err := ind.Update(ctx, in, &InputAcl{Interface: "loop300", Ip4Table: "w3-ip4"}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if err := ind.Delete(ctx, in, meta); err != nil {
		t.Fatal(err)
	}
	if inreq = v.CallsNamed("input_acl_set_interface")[1].(*classifyapi.InputACLSetInterface); inreq.IsAdd || inreq.L2TableIndex != 2 {
		t.Fatalf("delete request = %+v", inreq)
	}
	if actual, _ = ind.Retrieve(ctx); len(actual) != 0 {
		t.Fatalf("after Delete = %+v", actual)
	}

	// output acl (write-only)
	outd := NewOutputACL(v, "w3", store)
	out := &OutputAcl{Interface: "loop300", Ip6Table: "w3-ip6"}
	if k := outd.KeyOf(out); k != "classify.output-acl/loop300" || len(outd.Dependencies(out)) != 2 {
		t.Fatalf("KeyOf = %s", k)
	}
	if _, err := outd.Create(ctx, out); err != nil {
		t.Fatal(err)
	}
	outreq := v.CallsNamed("output_acl_set_interface")[0].(*classifyapi.OutputACLSetInterface)
	if !outreq.IsAdd || outreq.IP6TableIndex != 1 || outreq.IP4TableIndex != NoIndex {
		t.Fatalf("request = %+v", outreq)
	}
	if err := outd.Delete(ctx, out, BindingMeta{SwIfIndex: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := outd.Retrieve(ctx); !errors.Is(err, df2.ErrRetrieveUnsupported) {
		t.Fatalf("Retrieve: %v", err)
	}
	if err := outd.Delete(ctx, out, nil); !errors.Is(err, df2.ErrBadMeta) {
		t.Fatalf("bad meta: %v", err)
	}
}

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	Register(reg, fake.New(), "w3", nil)
	want := []string{TableName, SessionName, InterfaceIPTableName, InterfaceL2TablesName, InputACLName, OutputACLName}
	got := reg.Names()
	if len(got) != len(want) {
		t.Fatalf("Names = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names = %v", got)
		}
	}
}
