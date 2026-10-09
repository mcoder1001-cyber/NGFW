package ip6nd

import (
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	interfaces "ngfw/agent/binapi/interface"
	ip6api "ngfw/agent/binapi/ip6_nd"
	"ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
)

type rejectedRAClaims struct{}

func (rejectedRAClaims) Claim(string) error   { return errors.New("claim persistence unavailable") }
func (rejectedRAClaims) Release(string) error { return nil }
func (rejectedRAClaims) Claimed(string) bool  { return false }
func untaggedRAFixture() *fakeVPP {
	v := newFakeVPP()
	v.Reply("sw_interface_dump", &interfaces.SwInterfaceDetails{SwIfIndex: 5, InterfaceName: "lan0"})
	v.radvs[5] = defaultRadv(5)
	return v
}
func TestRaClaimFailurePrecedesMutation(t *testing.T) {
	for _, kind := range []string{"prefix", "configuration"} {
		t.Run(kind, func(t *testing.T) {
			v := untaggedRAFixture()
			var d scheduler.Descriptor
			var value proto.Message
			if kind == "prefix" {
				d = NewRaPrefix(v, "w3", df2.WithClaims(rejectedRAClaims{}))
				value = &RaPrefix{Interface: "lan0", Prefix: "2001:db8::/64", ValidLifetime: 3600, PreferredLifetime: 1800}
			} else {
				d = NewRaConfig(v, "w3", df2.WithClaims(rejectedRAClaims{}))
				value = &RaConfig{Interface: "lan0", Managed: true}
			}
			if _, err := d.Create(t.Context(), value); err == nil {
				t.Fatal("ignored unavailable ownership persistence")
			}
			if len(v.CallsNamed("sw_interface_ip6nd_ra_prefix"))+len(v.CallsNamed("sw_interface_ip6nd_ra_config")) != 0 {
				t.Fatal("mutated VPP before durable claim")
			}
		})
	}
}
func TestRaPrefixPartialFailureRetainsRollbackOwnership(t *testing.T) {
	v := untaggedRAFixture()
	claims := acl.NewMemoryClaimStore()
	d := NewRaPrefix(v, "w3", df2.WithClaims(claims))
	value := &RaPrefix{Interface: "lan0", Prefix: "2001:db8::/64", ValidLifetime: 3600, PreferredLifetime: 1800}
	// VPP accepts the prefix, but timer/readback capture becomes unavailable.
	v.On("sw_interface_ip6nd_ra_dump", func(api.Message) ([]api.Message, error) { return nil, errors.New("readback unavailable") })
	meta, err := d.Create(t.Context(), value)
	if !scheduler.IsPartialCreate(err) || meta != (RaMeta{SwIfIndex: 5}) || !claims.Claimed(string(d.KeyOf(value))) {
		t.Fatalf("lost partial mutation: meta=%v err=%v claimed=%v", meta, err, claims.Claimed(string(d.KeyOf(value))))
	}
	if len(v.radvs[5].prefixes) != 1 {
		t.Fatal("fixture did not produce a partial mutation")
	}
	// Rollback has the exact handle and durable authority even though capture failed.
	if err = d.Delete(t.Context(), value, meta); err != nil {
		t.Fatal(err)
	}
	if len(v.radvs[5].prefixes) != 0 || claims.Claimed(string(d.KeyOf(value))) {
		t.Fatal("rollback retained prefix or claim")
	}
}
func TestRaConfigPartialFailureRetainsRollbackOwnership(t *testing.T) {
	v := untaggedRAFixture()
	claims := acl.NewMemoryClaimStore()
	failed := false
	v.On("sw_interface_ip6nd_ra_config", func(message api.Message) ([]api.Message, error) {
		req := message.(*ip6api.SwInterfaceIP6ndRaConfig)
		code := v.radvs[5].apply(req)
		if !req.IsNo && req.Managed != 0 && !failed {
			failed = true
			return nil, errors.New("reply lost after mutation")
		}
		return []api.Message{&ip6api.SwInterfaceIP6ndRaConfigReply{Retval: code}}, nil
	})
	d := NewRaConfig(v, "w3", df2.WithClaims(claims))
	value := &RaConfig{Interface: "lan0", Managed: true}
	meta, err := d.Create(t.Context(), value)
	if !scheduler.IsPartialCreate(err) || meta != (RaMeta{SwIfIndex: 5}) || !claims.Claimed(string(d.KeyOf(value))) {
		t.Fatalf("partial=%v %v", meta, err)
	}
	if err = d.Delete(t.Context(), value, meta); err != nil {
		t.Fatal(err)
	}
	if v.radvs[5].det.AdvManagedFlag || claims.Claimed(string(d.KeyOf(value))) {
		t.Fatal("rollback did not restore defaults")
	}
}
