package l2_test

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	l2api "ngfw/agent/binapi/l2"
	"ngfw/agent/internal/descriptors/l2"
	"ngfw/agent/internal/scheduler"
)

// F-bridge-l2: the record name of a bridge domain travels in the tag ("<owner>:<id>/<name>") so
// Retrieve names the domain again; a rename is a recreate; the plain "<owner>:<id>" form still counts.
func TestBridgeDomainName(t *testing.T) {
	f := newFake()
	d := l2.NewBridgeDomain(f, owner)
	named := &l2.BridgeDomain{Id: 2010, Flood: true, Learn: true, Name: "lan"}
	meta := mustCreate(t, d, named)
	req := f.CallsNamed("bridge_domain_add_del_v2")[0].(*l2api.BridgeDomainAddDelV2)
	if req.BdTag != "w2:2010/lan" {
		t.Fatalf("tag = %q", req.BdTag)
	}
	assertOnly(t, d, map[scheduler.Key]proto.Message{"l2.bridge-domain/2010": named})
	if _, err := d.Update(ctx, named, &l2.BridgeDomain{Id: 2010, Flood: true, Learn: true, Name: "dmz"}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("rename: %v, want ErrRecreate", err)
	}
	if _, err := d.Create(ctx, &l2.BridgeDomain{Id: 2011, Name: "a/b"}); err == nil {
		t.Fatal("a name with '/' was accepted")
	}
	// a tag whose id part is not the bridge-domain id, or another owner's, is never ours
	for _, tag := range []string{"w2:2099/lan", "w3:2012/lan", "w2:", "w2:2012x"} {
		if _, ok := l2.ParseBDTag(tag, owner, 2012); ok {
			t.Errorf("ParseBDTag(%q) accepted", tag)
		}
	}
	if n, ok := l2.ParseBDTag("w2:2012/core\x00\x00", owner, 2012); !ok || n != "core" {
		t.Errorf("ParseBDTag(named) = %q, %v", n, ok)
	}
	if n, ok := l2.ParseBDTag("w2:2012", owner, 2012); !ok || n != "" {
		t.Errorf("ParseBDTag(plain) = %q, %v", n, ok)
	}
}
