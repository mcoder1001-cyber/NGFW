package scheduler

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
)

// stubDescriptor is the minimum Descriptor for registry tests.
type stubDescriptor struct{ name string }

func (s stubDescriptor) Name() string                                     { return s.name }
func (stubDescriptor) KeyOf(proto.Message) Key                            { return "" }
func (stubDescriptor) Dependencies(proto.Message) []Dependency            { return nil }
func (stubDescriptor) Create(context.Context, proto.Message) (any, error) { return nil, nil }
func (stubDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, nil
}
func (stubDescriptor) Delete(context.Context, proto.Message, any) error { return nil }
func (stubDescriptor) Retrieve(context.Context) ([]KV, error)           { return nil, nil }

func TestRegistryAddGetOrder(t *testing.T) {
	r := NewRegistry()
	for _, n := range []string{"interface.loopback", "interface.ip", "ip.route"} {
		if err := r.Add(stubDescriptor{n}); err != nil {
			t.Fatalf("Add(%q): %v", n, err)
		}
	}
	if r.Len() != 3 {
		t.Fatalf("Len = %d, want 3", r.Len())
	}
	got := r.Names()
	want := []string{"interface.loopback", "interface.ip", "ip.route"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names = %v, want %v (registration order)", got, want)
		}
	}
	if d, ok := r.Get("ip.route"); !ok || d.Name() != "ip.route" {
		t.Fatalf("Get(ip.route) = %v, %v", d, ok)
	}
	if _, ok := r.Get("nat44.pool"); ok {
		t.Fatal("Get of unknown name reported ok")
	}
	if d, ok := r.ForKey(Join("interface.ip", "loop200", "10.2.1.1/24")); !ok || d.Name() != "interface.ip" {
		t.Fatalf("ForKey = %v, %v", d, ok)
	}
	if ds := r.Descriptors(); len(ds) != 3 || ds[2].Name() != "ip.route" {
		t.Fatalf("Descriptors = %v", ds)
	}
}

func TestRegistryDuplicateAndInvalid(t *testing.T) {
	r := NewRegistry()
	if err := r.Add(stubDescriptor{"interface.loopback"}); err != nil {
		t.Fatal(err)
	}
	err := r.Add(stubDescriptor{"interface.loopback"})
	if !errors.Is(err, ErrDuplicateDescriptor) {
		t.Fatalf("duplicate: got %v, want ErrDuplicateDescriptor", err)
	}
	if err := r.Add(nil); !errors.Is(err, ErrNilDescriptor) {
		t.Fatalf("nil: got %v", err)
	}
	for _, bad := range []string{"", "Interface", "vpp/interface", "a..b", ".a", "a.", "-a", "a b", "1a"} {
		if err := r.Add(stubDescriptor{bad}); !errors.Is(err, ErrInvalidDescriptorName) {
			t.Fatalf("name %q: got %v, want ErrInvalidDescriptorName", bad, err)
		}
	}
	if r.Len() != 1 {
		t.Fatalf("failed adds must not register anything; Len = %d", r.Len())
	}
}

func TestRegisterPanicsOnDuplicate(t *testing.T) {
	r := NewRegistry()
	r.Register(stubDescriptor{"ip.route"})
	defer func() {
		v := recover()
		err, ok := v.(error)
		if !ok || !errors.Is(err, ErrDuplicateDescriptor) {
			t.Fatalf("Register duplicate: recovered %v, want ErrDuplicateDescriptor", v)
		}
	}()
	r.Register(stubDescriptor{"ip.route"})
	t.Fatal("Register did not panic on duplicate name")
}

func TestValidName(t *testing.T) {
	good := []string{"a", "interface.loopback", "nat44-ed.static-mapping", "ip6.route", "x.y.z"}
	for _, n := range good {
		if !ValidName(n) {
			t.Errorf("ValidName(%q) = false, want true", n)
		}
	}
	bad := []string{"", "A", "a/b", "a..b", ".a", "a.", "-a", "a-", "a-.b", "a b", "9", string(make([]byte, 65))}
	for _, n := range bad {
		if ValidName(n) {
			t.Errorf("ValidName(%q) = true, want false", n)
		}
	}
}

func TestKeyHelpers(t *testing.T) {
	k := Join("ip.route", "vrf2000", "10.2.0.0/24")
	if k != "ip.route/vrf2000/10.2.0.0/24" {
		t.Fatalf("Join = %q", k)
	}
	if k.Descriptor() != "ip.route" {
		t.Fatalf("Descriptor = %q", k.Descriptor())
	}
	if k.ID() != "vrf2000/10.2.0.0/24" {
		t.Fatalf("ID = %q", k.ID())
	}
	if Key("bare").Descriptor() != "bare" || Key("bare").ID() != "" {
		t.Fatal("key without separator must be all descriptor, empty id")
	}
	var p Plan
	if !p.Empty() || p.Len() != 0 {
		t.Fatal("zero Plan must be empty")
	}
	p.Delete = append(p.Delete, KV{Key: k})
	if p.Empty() || p.Len() != 1 {
		t.Fatal("Plan with one delete is not empty")
	}
}
