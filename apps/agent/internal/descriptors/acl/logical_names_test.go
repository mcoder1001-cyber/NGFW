package acl

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
)

// TestBindingLogicalNames (D-069): interfaces are named by their LOGICAL name — the owner-tag id
// for interfaces the agent created (VPP's name "tap1042" is index-based), VPP's name for untagged
// ones — resolved with DF-1's resolver. VPP's name of our own interface and another owner's
// interface are refused where the object is owned per interface (ethertype whitelist).
func TestBindingLogicalNames(t *testing.T) {
	ctx := t.Context()
	v := newFakeVPP()
	const tapIdx = 42
	v.addInterface(tapIdx, "tap1042", "w10:w10-tap42")
	acls := NewACL(v, owner)
	if _, err := acls.Create(ctx, ACL{Name: "a", Rules: sampleRules()[:1]}.Proto()); err != nil {
		t.Fatal(err)
	}
	d := NewInterfaceBinding(v, owner)
	b := InterfaceBinding{Interface: "w10-tap42", Input: []string{"a"}, Output: []string{}}
	meta, err := d.Create(ctx, b.Proto())
	if err != nil {
		t.Fatalf("binding by logical name: %v", err)
	}
	if meta.(BindingMeta).SwIfIndex != tapIdx {
		t.Fatalf("meta = %+v", meta)
	}
	if deps := d.Dependencies(b.Proto()); deps[0].Key != "interface/w10-tap42" {
		t.Fatalf("dependency = %+v, want the alias of the logical name", deps[0])
	}
	actual, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, kv := range actual {
		if kv.Key == "acl.interface-binding/w10-tap42" {
			found = proto.Equal(kv.Value, b.Proto())
		}
	}
	if !found {
		t.Fatalf("Retrieve does not report the binding under the logical name: %+v", actual)
	}
	if _, err := d.Create(ctx, InterfaceBinding{Interface: "tap1042", Input: []string{"a"}}.Proto()); !errors.Is(err, ErrNoInterface) {
		t.Fatalf("VPP's name of our interface must not resolve: %v", err)
	}
	// ACL lists are shared per interface (DF-4 finding 6): a binding may name another owner's
	// interface by its VPP name, an ethertype whitelist (owned per interface) may not
	if _, err := d.Create(ctx, InterfaceBinding{Interface: "loop300", Input: []string{"a"}}.Proto()); err != nil {
		t.Fatalf("binding on another owner's interface: %v", err)
	}
	if _, err := NewEtypeWhitelist(v, owner).Create(ctx, EtypeWhitelist{Interface: "loop300", Input: []uint16{0x0800}}.Proto()); !errors.Is(err, ErrForeignInterface) {
		t.Fatalf("whitelist on another owner's interface must be refused: %v", err)
	}
	// an untagged (physical) port keeps its VPP name as logical name
	if _, err := d.Create(ctx, InterfaceBinding{Interface: "host-eth0", Input: []string{"a"}}.Proto()); err != nil {
		t.Fatalf("untagged interface by VPP name: %v", err)
	}
}
