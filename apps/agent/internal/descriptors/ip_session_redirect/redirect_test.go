package sessionredirect

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	classifyapi "ngfw/agent/binapi/classify"
	"ngfw/agent/binapi/fib_types"
	interfaces "ngfw/agent/binapi/interface"
	isr "ngfw/agent/binapi/ip_session_redirect"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/classify"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

type fakeVPP struct {
	*fake.Client
	redirects []isr.IPSessionRedirectDetails
}

func newFakeVPP() *fakeVPP {
	v := &fakeVPP{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{}))}
	v.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 5, InterfaceName: "loop300", Tag: "w3:loop300"},
	)
	v.Reply("classify_table_ids", &classifyapi.ClassifyTableIdsReply{Ids: []uint32{0, 1, 2}})
	v.On("ip_session_redirect_add_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*isr.IPSessionRedirectAddV2)
		det := isr.IPSessionRedirectDetails{TableIndex: r.TableIndex, OpaqueIndex: r.OpaqueIndex, IsPunt: r.IsPunt,
			IsIP6: r.Proto == fib_types.FIB_API_PATH_NH_PROTO_IP6, MatchLength: uint32(r.MatchLen), Match: append([]byte(nil), r.Match...), Paths: r.Paths}
		for i, e := range v.redirects {
			if e.TableIndex == r.TableIndex && bytes.Equal(e.Match, r.Match) {
				v.redirects[i] = det
				return []api.Message{&isr.IPSessionRedirectAddV2Reply{}}, nil
			}
		}
		v.redirects = append(v.redirects, det)
		return []api.Message{&isr.IPSessionRedirectAddV2Reply{}}, nil
	})
	v.On("ip_session_redirect_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*isr.IPSessionRedirectDel)
		for i, e := range v.redirects {
			if e.TableIndex == r.TableIndex && bytes.Equal(e.Match, r.Match) {
				v.redirects = append(v.redirects[:i], v.redirects[i+1:]...)
				return []api.Message{&isr.IPSessionRedirectDelReply{}}, nil
			}
		}
		return []api.Message{&isr.IPSessionRedirectDelReply{Retval: -6}}, nil
	})
	v.On("ip_session_redirect_dump", func(req api.Message) ([]api.Message, error) {
		want := req.(*isr.IPSessionRedirectDump).TableIndex
		var out []api.Message
		for i := range v.redirects {
			if v.redirects[i].TableIndex == want {
				d := v.redirects[i]
				out = append(out, &d)
			}
		}
		return out, nil
	})
	return v
}

func TestLifecycle(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	store := classify.NewMemStore()
	_ = store.Put(classify.TableRecord{Name: "w3-t1", Index: 1, SkipNVectors: 1, MatchNVectors: 2})
	// A redirect in a table that is not ours (index 2) must stay invisible.
	v.redirects = []isr.IPSessionRedirectDetails{{TableIndex: 2, MatchLength: 16, Match: []byte{9}}}
	d := New(v, "w3", store)
	if !scheduler.ValidName(d.Name()) {
		t.Fatal(d.Name())
	}
	match := append(make([]byte, 16), 0x0a, 0x03, 0x00, 0x01) // skip vector zero, then the key
	desired := &Redirect{Table: "w3-t1", Match: append(append([]byte(nil), match...), 0, 0, 0), OpaqueIndex: 7,
		Paths: []*df2.FibPath{{NextHop: "10.3.1.254", Interface: "loop300"}}}
	if k := d.KeyOf(desired); k != "ip-session-redirect.redirect/w3-t1/000000000000000000000000000000000a030001" {
		t.Fatalf("KeyOf = %s", k)
	}
	deps := d.Dependencies(desired)
	if len(deps) != 2 || deps[0].Key != "classify.table/w3-t1" || deps[0].Optional || deps[1].Key != "interface/loop300" || !deps[1].Optional {
		t.Fatalf("Dependencies = %+v", deps)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil || meta != (Meta{TableIndex: 1}) {
		t.Fatalf("Create: %v %+v", err, meta)
	}
	req := v.CallsNamed("ip_session_redirect_add_v2")[0].(*isr.IPSessionRedirectAddV2)
	if req.TableIndex != 1 || req.MatchLen != 48 || len(req.Match) != 48 || !bytes.Equal(req.Match[:20], match) || req.OpaqueIndex != 7 ||
		req.Proto != fib_types.FIB_API_PATH_NH_PROTO_IP4 || req.IsPunt || req.NPaths != 1 || req.Paths[0].SwIfIndex != 5 {
		t.Fatalf("request = %+v", req)
	}
	norm, _ := Normalize(desired)
	actual, err := d.Retrieve(ctx)
	if err != nil || len(actual) != 1 || actual[0].Key != d.KeyOf(desired) || !proto.Equal(actual[0].Value, norm) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v, %v\nwant %+v", actual, err, norm)
	}
	// Every change is a recreate (VPP rejects re-adding an existing session).
	updated := &Redirect{Table: "w3-t1", Match: match, Punt: true, Paths: desired.Paths}
	if _, err := d.Update(ctx, desired, updated, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	del := v.CallsNamed("ip_session_redirect_del")[0].(*isr.IPSessionRedirectDel)
	if del.TableIndex != 1 || del.MatchLen != 48 {
		t.Fatalf("delete request = %+v", del)
	}
	if actual, _ = d.Retrieve(ctx); len(actual) != 0 || len(v.redirects) != 1 {
		t.Fatalf("after Delete = %+v redirects=%d", actual, len(v.redirects))
	}
	// Errors.
	if _, err := d.Create(ctx, &Redirect{Table: "nope", Match: match, Punt: true}); !errors.Is(err, classify.ErrNoSuchTable) {
		t.Fatalf("unknown table: %v", err)
	}
	if _, err := d.Create(ctx, &Redirect{Table: "w3-t1", Match: match, Punt: true}); err == nil {
		t.Fatal("no paths accepted")
	}
	if _, err := d.Create(ctx, &Redirect{Table: "w3-t1", Match: append(make([]byte, 48), 1), Punt: true}); err == nil {
		t.Fatal("match longer than the table geometry accepted")
	}
	if err := d.Delete(ctx, desired, meta); err == nil {
		t.Fatal("deleting a missing redirect must surface the retval")
	}
	if err := d.Delete(ctx, desired, nil); !errors.Is(err, df2.ErrBadMeta) {
		t.Fatalf("bad meta: %v", err)
	}
	// A table larger than the API's 80-byte match is rejected.
	_ = store.Put(classify.TableRecord{Name: "w3-big", Index: 0, SkipNVectors: 2, MatchNVectors: 4})
	if _, err := d.Create(ctx, &Redirect{Table: "w3-big", Match: match, Paths: desired.Paths}); err == nil {
		t.Fatal("96-byte geometry accepted")
	}
}

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	Register(reg, fake.New(), "w3", classify.NewMemStore())
	if got := reg.Names(); len(got) != 1 || got[0] != Name {
		t.Fatalf("Names = %v", got)
	}
}
