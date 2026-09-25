package subsystems

import (
	"context"
	"sync"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	abfapi "ngfw/agent/binapi/abf"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/abf"
	descacl "ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
)

type kvSink struct{ kvs []scheduler.KV }

func (s *kvSink) Add(k scheduler.Key, v proto.Message, _ string) {
	s.kvs = append(s.kvs, scheduler.KV{Key: k, Value: v})
}
func (*kvSink) Errorf(string, string, string, ...any) {}
func (*kvSink) Warnf(string, string, string, ...any)  {}

// D-131: a PBR (ABF) policy that names an F-acl list applies against the real acl.acl registration
// (F-rpf-adl-pbr's read-only pbr.acl-ref stand-in is no longer needed once acl.acl is registered): the
// scheduler orders the ACL before the policy (mandatory dependency acl.acl/<name>) and the policy is
// created with the ACL's VPP index (acl.LookupIndex, D-066).
func TestPBRPolicyNamesFACLList(t *testing.T) {
	v := coretest.New()
	var mu sync.Mutex
	policies := map[uint32]abfapi.AbfPolicy{}
	v.On("abf_policy_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*abfapi.AbfPolicyAddDel)
		mu.Lock()
		defer mu.Unlock()
		if r.IsAdd {
			policies[r.Policy.PolicyID] = r.Policy
		} else {
			delete(policies, r.Policy.PolicyID)
		}
		return []api.Message{&abfapi.AbfPolicyAddDelReply{}}, nil
	})
	v.On("abf_policy_dump", func(api.Message) ([]api.Message, error) {
		mu.Lock()
		defer mu.Unlock()
		var out []api.Message
		for _, p := range policies {
			out = append(out, &abfapi.AbfPolicyDetails{Policy: p})
		}
		return out, nil
	})
	dir := t.TempDir()
	owned, err := ownertable.Open(dir, "w3")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvDNSServers, "127.0.0.1:9")
	reg := scheduler.NewRegistry()
	w, err := Register(reg, Env{Client: v, Owner: "w3", StateDir: dir, Owned: owned})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	w.Connected(context.Background())
	pol := abf.NewPolicy(v, "w3", nil)
	reg.Register(pol)

	sink := &kvSink{}
	desired.ACL(sink, &vrxv1.DesiredState{Acl: &vrxv1.AclConfig{Lists: map[string]*vrxv1.AclList{"pbr-match": {Rules: []*vrxv1.AclRule{
		{Sequence: proto.Uint32(10), Action: proto.String("permit"), IpVersion: proto.String("ipv4"), Source: &vrxv1.AddressMatch{Kind: proto.String("prefix"), Prefix: proto.String("10.3.1.0/24")}},
	}}}}}, map[string]bool{"acl": true})
	p, err := abf.NormalizePolicy(&abf.Policy{PolicyId: 3001, Acl: "pbr-match", Paths: []*df2.FibPath{{NextHop: "10.3.2.2", Weight: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	kvs := append(sink.kvs, scheduler.KV{Key: pol.KeyOf(p), Value: p})
	sched := scheduler.New(reg, nil)
	res := sched.ApplyWith(context.Background(), kvs, scheduler.Only(append(aclDescriptors(), abf.PolicyName)...), scheduler.ApplyOptions{})
	if res.Err != nil || res.Outcome != scheduler.OutcomeApplied {
		t.Fatalf("apply: %v %v", res.Outcome, res.Err)
	}
	var aclIdx uint32 = ^uint32(0)
	for idx, tag := range v.ACL().ACLs() {
		if tag == "w3:pbr-match" {
			aclIdx = idx
		}
	}
	mu.Lock()
	got, ok := policies[3001]
	mu.Unlock()
	if !ok || got.ACLIndex != aclIdx || aclIdx == ^uint32(0) {
		t.Fatalf("ABF policy %+v (ok %v), want acl_index %d of w3:pbr-match", got, ok, aclIdx)
	}
	order := map[scheduler.Key]int{}
	for i, r := range res.Results {
		order[r.Key] = i
	}
	if order[descacl.KeyACL("pbr-match")] > order[pol.KeyOf(p)] {
		t.Fatalf("the ACL must be created before the policy: %v", res.Results)
	}
}
