package hoststack

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	httpstatic "ngfw/agent/binapi/http_static"
	"ngfw/agent/binapi/session"
	"ngfw/agent/binapi/tcp"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/scheduler"
)

// model is the fake VPP's host stack.
type model struct {
	sessionOn bool
	engine    session.RtBackendEngine
	nsNext    uint32
	ns        map[string]uint32
	rules     map[string]*session.SessionRuleAddDel // by match (tag ignored, as VPP)
	tcpAdds   int
	httpAdds  int
}

func matchKey(r *session.SessionRuleAddDel) string {
	c := *r
	c.IsAdd, c.Tag, c.ActionIndex = false, "", 0
	return c.Lcl.String() + "|" + c.Rmt.String() + "|" + string(rune(c.LclPort)) + "|" + string(rune(c.RmtPort)) + "|" + c.TransportProto.String() + "|" + c.Scope.String() + "|" + string(rune(c.AppnsIndex))
}

func newFake(on bool) (*dfkittest.FakeVPP, *model) {
	f := dfkittest.NewFake(dfkittest.Iface{Index: 1, Name: "loop13", Tag: "w13:loop13"})
	m := &model{sessionOn: on, nsNext: 1, ns: map[string]uint32{}, rules: map[string]*session.SessionRuleAddDel{}}
	f.On("session_enable_disable_v2", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*session.SessionEnableDisableV2)
		m.sessionOn = r.RtEngineType != session.RT_BACKEND_ENGINE_API_DISABLE
		m.engine = r.RtEngineType
		return []api.Message{&session.SessionEnableDisableV2Reply{}}, nil
	})
	f.On("session_rules_v2_dump", func(api.Message) ([]api.Message, error) {
		if !m.sessionOn {
			return nil, api.VPPApiError(api.FEATURE_DISABLED)
		}
		var out []api.Message
		for _, r := range m.rules {
			out = append(out, &session.SessionRulesV2Details{TransportProto: r.TransportProto, Lcl: r.Lcl, Rmt: r.Rmt,
				LclPort: r.LclPort, RmtPort: r.RmtPort, ActionIndex: r.ActionIndex, Scope: r.Scope, Tag: r.Tag,
				Count: 1, AppnsIndex: []uint32{r.AppnsIndex}})
		}
		return out, nil
	})
	f.On("app_namespace_add_del_v4", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*session.AppNamespaceAddDelV4)
		if !m.sessionOn {
			return []api.Message{&session.AppNamespaceAddDelV4Reply{Retval: int32(api.FEATURE_DISABLED)}}, nil
		}
		if !r.IsAdd {
			if _, ok := m.ns[r.NamespaceID]; !ok {
				return []api.Message{&session.AppNamespaceAddDelV4Reply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil
			}
			delete(m.ns, r.NamespaceID)
			return []api.Message{&session.AppNamespaceAddDelV4Reply{}}, nil
		}
		idx, ok := m.ns[r.NamespaceID]
		if !ok {
			idx = m.nsNext
			m.nsNext++
			m.ns[r.NamespaceID] = idx
		}
		return []api.Message{&session.AppNamespaceAddDelV4Reply{AppnsIndex: idx}}, nil
	})
	f.On("session_rule_add_del", func(msg api.Message) ([]api.Message, error) {
		r := msg.(*session.SessionRuleAddDel)
		k := matchKey(r)
		if r.IsAdd {
			c := *r
			m.rules[k] = &c // an identical match is replaced (VPP)
		} else {
			if _, ok := m.rules[k]; !ok {
				return []api.Message{&session.SessionRuleAddDelReply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil
			}
			delete(m.rules, k)
		}
		return []api.Message{&session.SessionRuleAddDelReply{}}, nil
	})
	f.On("tcp_configure_src_addresses", func(api.Message) ([]api.Message, error) {
		m.tcpAdds++
		return []api.Message{&tcp.TCPConfigureSrcAddressesReply{}}, nil
	})
	f.On("http_static_enable_v5", func(api.Message) ([]api.Message, error) {
		m.httpAdds++
		return []api.Message{&httpstatic.HTTPStaticEnableV5Reply{}}, nil
	})
	return f, m
}

const owner = "w13"

var (
	ns    = Namespace{ID: "w13-app", Interface: "loop13", Vrf: 0}
	rule1 = Rule{Tag: "w13-deny", Scope: "global", Transport: "tcp", Local: "10.13.1.0/24", LocalPort: 31390,
		Remote: "10.13.2.0/24", Action: "deny"}
	rule2 = Rule{Tag: "w13-allow6", Scope: "local", Transport: "udp", Local: "fd00:13::/48", Remote: "::/0",
		RemotePort: 31391, Action: "allow", AppNamespace: "w13-app"}
)

func descs(t *testing.T, f *dfkittest.FakeVPP, boot dfkit.BootStore) (*NamespaceDescriptor, *RuleDescriptor) {
	t.Helper()
	r := scheduler.NewRegistry()
	Register(r, f, owner, WithBootStore(boot))
	want := []string{NameSession, NameNamespace, NameSessionRule, NameTCPSrc}
	if got := r.Names(); len(got) != len(want) {
		t.Fatalf("registered %v, want %v", got, want)
	}
	st, _ := lookupState(owner)
	return newNamespace(f, owner, st), newRule(f, owner, st)
}

func TestNamespaceAndRulesLifecycle(t *testing.T) {
	f, m := newFake(true)
	ctx := context.Background()
	boot := dfkit.NewMemoryBootStore()
	nd, rd := descs(t, f, boot)

	for i := 0; i < 2; i++ { // duplicate add is idempotent
		if _, err := nd.Create(ctx, ns.Proto()); err != nil {
			t.Fatalf("namespace create #%d: %v", i, err)
		}
		for _, r := range []Rule{rule1, rule2} {
			if _, err := rd.Create(ctx, r.Proto()); err != nil {
				t.Fatalf("rule create #%d %s: %v", i, r.Tag, err)
			}
		}
	}
	if len(m.ns) != 1 || len(m.rules) != 2 {
		t.Fatalf("VPP has %d namespaces, %d rules", len(m.ns), len(m.rules))
	}
	if _, err := nd.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
		t.Fatalf("namespace Retrieve must be write-only: %v", err)
	}
	// a foreign rule is never reported
	m.rules["foreign"] = &session.SessionRuleAddDel{Tag: "w12:x", ActionIndex: actionDrop}
	dfkittest.AssertEmptyPlan(t, rd, dfkittest.KV(rd, rule1.Proto()), dfkittest.KV(rd, rule2.Proto()))

	snap, err := State(ctx, f, owner)
	if err != nil || !snap.SessionEnabled || len(snap.Rules) != 2 || snap.RuleCountTotal != 3 || len(snap.Namespaces) != 1 {
		t.Fatalf("state %+v %v", snap, err)
	}

	// agent restart: a new descriptor set over the same boot store sees the same state
	nd2, rd2 := descs(t, f, boot)
	dfkittest.AssertEmptyPlan(t, rd2, dfkittest.KV(rd2, rule1.Proto()), dfkittest.KV(rd2, rule2.Proto()))

	// update: action change replaces the rule
	upd := rule1
	upd.Action = "allow"
	if _, err := rd2.Update(ctx, rule1.Proto(), upd.Proto(), nil); err != nil {
		t.Fatal(err)
	}
	dfkittest.AssertRetrieved(t, rd2, dfkittest.KV(rd2, upd.Proto()))

	// rollback: rules then namespace; deleting twice is fine
	for i := 0; i < 2; i++ {
		for _, r := range []Rule{upd, rule2} {
			if err := rd2.Delete(ctx, r.Proto(), nil); err != nil {
				t.Fatalf("rule delete #%d: %v", i, err)
			}
		}
		if err := nd2.Delete(ctx, ns.Proto(), nil); err != nil {
			t.Fatalf("namespace delete #%d: %v", i, err)
		}
	}
	delete(m.rules, "foreign")
	if len(m.ns) != 0 || len(m.rules) != 0 {
		t.Fatalf("left behind: %v %v", m.ns, m.rules)
	}
	if kvs := dfkittest.MustRetrieve(t, rd2); len(kvs) != 0 {
		t.Fatalf("retrieve after rollback: %v", kvs)
	}
}

func TestRuleValidation(t *testing.T) {
	f, _ := newFake(true)
	_, rd := descs(t, f, dfkit.NewMemoryBootStore())
	ctx := context.Background()
	for name, r := range map[string]Rule{
		"non-canonical": {Tag: "a", Scope: "global", Transport: "tcp", Local: "10.0.0.1/24", Remote: "0.0.0.0/0", Action: "deny"},
		"mixed family":  {Tag: "a", Scope: "global", Transport: "tcp", Local: "10.0.0.0/24", Remote: "::/0", Action: "deny"},
		"dotdot tag":    {Tag: "a..b", Scope: "global", Transport: "tcp", Local: "10.0.0.0/24", Remote: "0.0.0.0/0", Action: "deny"},
		"bad action":    {Tag: "a", Scope: "global", Transport: "tcp", Local: "10.0.0.0/24", Remote: "0.0.0.0/0", Action: "drop"},
	} {
		if _, err := rd.Create(ctx, r.Proto()); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// a rule whose namespace is not applied fails clearly
	if _, err := rd.Create(ctx, rule2.Proto()); err == nil {
		t.Fatal("rule with unapplied namespace accepted")
	}
}

func TestSessionGlobals(t *testing.T) {
	ctx := context.Background()
	on := Session{Enabled: true}.Proto()

	// non-owner: require only, never sends session_enable_disable_v2
	f, m := newFake(true)
	d := NewSession(f, dfkit.GlobalsOwner(false))
	if _, err := d.Create(ctx, on); err != nil {
		t.Fatalf("non-owner, layer on: %v", err)
	}
	m.sessionOn = false
	if _, err := d.Create(ctx, on); !errors.Is(err, dfkit.ErrNotGlobalsOwner) {
		t.Fatalf("non-owner, layer off: %v", err)
	}
	if err := d.Delete(ctx, on, nil); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Calls() {
		if c.GetMessageName() == "session_enable_disable_v2" {
			t.Fatal("non-owner set the session layer")
		}
	}

	// owner: enables rule-table only when off; never re-engines, never disables
	f, m = newFake(false)
	d = NewSession(f, dfkit.GlobalsOwner(true))
	if _, err := d.Create(ctx, on); err != nil || !m.sessionOn || m.engine != session.RT_BACKEND_ENGINE_API_RULE_TABLE {
		t.Fatalf("owner enable: %v on=%v engine=%v", err, m.sessionOn, m.engine)
	}
	m.engine = session.RT_BACKEND_ENGINE_API_SDL
	if _, err := d.Create(ctx, on); err != nil || m.engine != session.RT_BACKEND_ENGINE_API_SDL {
		t.Fatalf("owner re-engined an enabled layer: %v %v", err, m.engine)
	}
	if err := d.Delete(ctx, on, nil); err != nil || !m.sessionOn {
		t.Fatalf("owner delete must leave the layer: %v %v", err, m.sessionOn)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
		t.Fatal(err)
	}
}

func TestTCPSrcAppliedOnce(t *testing.T) {
	f, m := newFake(true)
	ctx := context.Background()
	boot := dfkit.NewMemoryBootStore()
	d := NewTCPSrc(f, boot)
	v := TCPSrc{First: "10.13.1.10", Last: "10.13.1.20", Vrf: 13000}.Proto()
	for i := 0; i < 3; i++ {
		if _, err := d.Create(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	if m.tcpAdds != 1 {
		t.Fatalf("adds on one boot: %d", m.tcpAdds)
	}
	f.RestartVPP()
	if _, err := d.Create(ctx, v); err != nil || m.tcpAdds != 2 {
		t.Fatalf("after VPP restart: %v adds=%d", err, m.tcpAdds)
	}
	if err := d.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := boot.Get(string(KeyTCPSrc(13000))); ok {
		t.Fatal("record kept after delete")
	}
	if _, err := d.Create(ctx, TCPSrc{First: "10.0.0.9", Last: "10.0.0.1"}.Proto()); !errors.Is(err, dfkit.ErrSpec) {
		t.Fatalf("reversed range: %v", err)
	}
}

func TestHTTPStaticOptIn(t *testing.T) {
	f, m := newFake(true)
	ctx := context.Background()
	r := scheduler.NewRegistry()
	t.Setenv(EnvHTTPStatic, "")
	RegisterGlobals(r, f)
	if got := r.Names(); len(got) != 1 || got[0] != NameSession {
		t.Fatalf("default RegisterGlobals: %v", got)
	}
	r = scheduler.NewRegistry()
	t.Setenv(EnvHTTPStatic, "1")
	RegisterGlobals(r, f)
	if len(r.Names()) != 2 {
		t.Fatalf("opt-in RegisterGlobals: %v", r.Names())
	}
	d := NewHTTPStatic(f, dfkit.NewMemoryBootStore())
	for _, bad := range []string{"/etc", "/var/lib/vrx/www/../x", "/var/lib/vrx/www/a\x01"} {
		if _, err := d.Create(ctx, HTTPStatic{WWWRoot: bad, URI: "tcp://0.0.0.0/80"}.Proto()); !errors.Is(err, dfkit.ErrSpec) {
			t.Fatalf("www root %q: %v", bad, err)
		}
	}
	good := HTTPStatic{WWWRoot: "/var/lib/vrx/www/site", URI: "tcp://0.0.0.0/80", CacheSizeMB: 10}.Proto()
	for i := 0; i < 2; i++ {
		if _, err := d.Create(ctx, good); err != nil {
			t.Fatal(err)
		}
	}
	if m.httpAdds != 1 {
		t.Fatalf("http_static enabled %d times", m.httpAdds)
	}
	changed := proto.Clone(good)
	if _, err := d.Update(ctx, good, changed, nil); err == nil {
		t.Fatal("update of a running http_static accepted")
	}
}

func TestStateSessionOff(t *testing.T) {
	f, _ := newFake(false)
	snap, err := State(context.Background(), f, "nobody")
	if err != nil || snap.SessionEnabled || snap.SessionDetail == "" {
		t.Fatalf("%+v %v", snap, err)
	}
}
