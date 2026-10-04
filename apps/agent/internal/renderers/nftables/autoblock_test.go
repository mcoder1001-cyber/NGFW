package nftables

import (
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"strings"
	"testing"
)

func TestAutoBlockScanObserver(t *testing.T) {
	cfg := &ngfwv1.AutoBlock{Enabled: proto.Bool(true), Rules: []*ngfwv1.AutoBlockRule{{Source: proto.String("portScan"), Enabled: proto.Bool(true)}}}
	in := Input{AutoBlock: cfg, ACL: &ngfwv1.AclConfig{GlobalBlocking: &ngfwv1.GlobalBlocking{Lists: map[string]*ngfwv1.GlobalBlockingList{"auto-block": {ProtectHost: proto.Bool(true), Entries: []string{"192.0.2.7/32"}}}}}}
	v, issues := Build(in)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	b, err := RenderText("ngfw", v)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `log prefix "ngfw:scan "`) || v.Chains[0].Priority != -300 || v.Chains[1].Priority != -299 {
		t.Fatal("scan order/rules unsafe")
	}
	cfg.Enabled = proto.Bool(false)
	in.ACL = nil
	v, _ = Build(in)
	if v != nil {
		t.Fatal("disabled detector left observer")
	}
}
