package nftables

import (
	"bytes"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/subsystems/ruleexpiry"
)

// F-rule-expiry: an expired host rule is not rendered (a rule.expired warning at its pointer; the value still carries
// the configuration), a future expiry is rendered and noted.
func TestHostRuleExpiry(t *testing.T) {
	ruleexpiry.Reset()
	t.Cleanup(ruleexpiry.Reset)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	prev := ruleexpiry.Now
	ruleexpiry.Now = func() time.Time { return now }
	t.Cleanup(func() { ruleexpiry.Now = prev })

	ds := doc(t, fullDoc)
	rules := ds.GetAcl().GetHost()["local-in"].GetRules()
	rules[5].ExpiresAt = proto.String("2026-09-27T11:00:00Z") // seq 30: ssh to 10.9.77.1 — expired
	rules[4].ExpiresAt = proto.String("2026-09-30T00:00:00Z") // seq 20 — later
	v, issues := build(t, ds)
	var expired []Issue
	for _, is := range issues {
		if is.Rule == "rule.expired" {
			expired = append(expired, is)
		}
	}
	if len(expired) != 1 || expired[0].Pointer != "/acl/host/local-in/rules/5" || !expired[0].Warning {
		t.Fatalf("issues %+v", issues)
	}
	text, err := RenderText("ngfw_w9", v)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(text, []byte("10.9.77.1")) {
		t.Fatalf("the expired rule is rendered:\n%s", text)
	}
	if !proto.Equal(v.GetConfig().GetHost()["local-in"], ds.GetAcl().GetHost()["local-in"]) {
		t.Fatal("the value must still carry the configured list (Retrieve keeps the expired rule)")
	}
	if p := ruleexpiry.Pending(); len(p) != 1 || !p[0].Equal(time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("noted %v", p)
	}
}
