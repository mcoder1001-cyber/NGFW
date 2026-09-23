package abf

import (
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/descriptors/df2/df2test"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

func find(kvs []scheduler.KV, k scheduler.Key) *scheduler.KV {
	for i := range kvs {
		if kvs[i].Key == k {
			return &kvs[i]
		}
	}
	return nil
}

func TestABFOnHost(t *testing.T) {
	c := df2test.Connect(t)
	ctx := df2test.Ctx(t)
	owner := vpptest.Prefix(t)
	slot := df2test.Slot(t)
	ids := &df2.IDRange{Lo: vpptest.TableBase(t), Hi: vpptest.TableBase(t) + 999}
	in, inIdx := df2test.Loopback(t, c, 7)
	out, outIdx := df2test.Loopback(t, c, 8)
	df2test.AddAddress(t, c, inIdx, fmt.Sprintf("10.%d.7.1/24", slot))
	df2test.AddAddress(t, c, outIdx, fmt.Sprintf("10.%d.8.1/24", slot))
	aclIndex := df2test.ACL(t, c, "abf-test")

	pd := NewPolicy(c, owner, ids)
	policy := &Policy{PolicyId: vpptest.TableBase(t) + 1, Acl: "abf-test", Paths: []*df2.FibPath{{NextHop: fmt.Sprintf("10.%d.8.254", slot), Interface: out}}}
	pmeta, err := pd.Create(ctx, policy)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pd.Delete(df2test.Ctx(t), policy, pmeta) })
	if pmeta != (PolicyMeta{ACLIndex: aclIndex}) {
		t.Fatalf("policy meta = %+v, want acl %d", pmeta, aclIndex)
	}
	actual, err := pd.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kv := find(actual, pd.KeyOf(policy))
	if kv == nil {
		t.Fatalf("Retrieve does not show %s: %+v", pd.KeyOf(policy), actual)
	}
	t.Logf("abf policy Retrieve = %+v", kv.Value)
	want, _ := NormalizePolicy(policy)
	if !proto.Equal(kv.Value, want) || kv.Meta != pmeta {
		t.Fatalf("policy Retrieve = %+v, want %+v", kv.Value, want)
	}

	ad := NewAttach(c, owner, ids)
	attach := &Attach{PolicyId: policy.PolicyId, Interface: in, Priority: 10}
	ameta, err := ad.Create(ctx, attach)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ad.Delete(df2test.Ctx(t), attach, ameta) })
	aactual, err := ad.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	akv := find(aactual, ad.KeyOf(attach))
	if akv == nil || !proto.Equal(akv.Value, attach) || akv.Meta != ameta {
		t.Fatalf("attach Retrieve = %+v, want %+v", aactual, attach)
	}
	t.Logf("abf attach Retrieve = %+v", akv.Value)

	// Path update in place (additive API: add the new path, remove the old one).
	updated := &Policy{PolicyId: policy.PolicyId, Acl: "abf-test", Paths: []*df2.FibPath{{Type: df2.FibPath_DROP}}}
	if _, err := pd.Update(ctx, policy, updated, pmeta); err != nil {
		t.Fatal(err)
	}
	actual, _ = pd.Retrieve(ctx)
	wantUpd, _ := NormalizePolicy(updated)
	if kv = find(actual, pd.KeyOf(policy)); kv == nil || !proto.Equal(kv.Value, wantUpd) {
		t.Fatalf("after Update = %+v, want %+v", kv, wantUpd)
	}
	df2test.Hold(t)
	if err := ad.Delete(ctx, attach, ameta); err != nil {
		t.Fatal(err)
	}
	if err := pd.Delete(ctx, updated, pmeta); err != nil {
		t.Fatal(err)
	}
	if aactual, _ = ad.Retrieve(ctx); find(aactual, ad.KeyOf(attach)) != nil {
		t.Fatalf("attach still retrieved: %+v", aactual)
	}
	if actual, _ = pd.Retrieve(ctx); find(actual, pd.KeyOf(policy)) != nil {
		t.Fatalf("policy still retrieved: %+v", actual)
	}
}
