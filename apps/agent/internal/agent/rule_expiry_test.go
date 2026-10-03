package agent

// F-rule-expiry acceptance on the fake VPP: a rule with expiresAt is rendered, removed from VPP at that instant
// without a commit (the ruleexpiry watcher asks for a resync), kept in the configuration (Retrieve), and not re-installed
// by an agent restart.

import (
	"context"
	"fmt"
	"testing"
	"time"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/subsystems/ruleexpiry"
)

func TestACLRuleRemovedAtExpiryWithoutACommit(t *testing.T) {
	ruleexpiry.Reset()
	t.Cleanup(ruleexpiry.Reset)
	v := coretest.New()
	dir := t.TempDir()
	s, _ := newACLSvc(t, v, dir, false)
	at := time.Now().Add(1500 * time.Millisecond).UTC().Format(time.RFC3339Nano)
	d := doc(t, fmt.Sprintf(`{
  "vrfs": {"default": {"id": 0}},
  "interfaces": {"loop701": {"enabled": true, "vrf": "default"}},
  "acl": {
    "lists": {"tmp": {"tags": [], "rules": [
      {"sequence": 10, "action": "permit", "enabled": true, "ipVersion": "ipv4", "source": {"kind": "prefix", "prefix": "10.7.1.0/24"}, "log": false,
       "expiresAt": %q, "owner": "netops", "ticket": "CHG-1"},
      {"sequence": 20, "action": "deny", "enabled": true, "ipVersion": "ipv4", "log": false}
    ]}},
    "attachments": [{"list": "tmp", "target": {"kind": "interface", "interface": "loop701"}, "direction": "in", "sequence": 1, "enabled": true}]
  }
}`, at))
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "a1", DesiredState: d}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	idx := ownedACL(t, v, "tmp")
	if n := len(v.ACL().Rules(idx)); n != 2 {
		t.Fatalf("before expiry: %d VPP rules, want 2", n)
	}

	// no commit: the watcher's resync removes rule 10 at its expiry
	deadline := time.Now().Add(10 * time.Second)
	for len(v.ACL().Rules(ownedACL(t, v, "tmp"))) != 1 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if rules := v.ACL().Rules(ownedACL(t, v, "tmp")); len(rules) != 1 || rules[0].IsPermit != 0 {
		t.Fatalf("after expiry: VPP rules %+v, want only the deny", rules)
	}
	// the configuration keeps the rule (extend or delete it): Retrieve reports the applied list as configured
	got := retrieveACL(t, s)
	if r := got.GetLists()["tmp"].GetRules(); len(r) != 2 || r[0].GetExpiresAt() != at || r[0].GetOwner() != "netops" || r[0].GetTicket() != "CHG-1" {
		t.Fatalf("Retrieve lost the expired rule or its metadata: %v", got.GetLists()["tmp"])
	}

	// agent restart: the first resync does not re-install it
	s.Close()
	v2 := v
	s2, _ := newACLSvc(t, v2, dir, false)
	mustStatus(t, s2.Resync(context.Background()), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if rules := v.ACL().Rules(ownedACL(t, v, "tmp")); len(rules) != 1 {
		t.Fatalf("after an agent restart: VPP rules %+v, want the expired rule still out", rules)
	}
}
