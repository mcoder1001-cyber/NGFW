package subsystems

// F-rule-expiry wiring: the watcher of ruleexpiry asks for a resync of the stored desired state when the earliest
// expiresAt of a rendered rule (ACL rule, host rule, NAT static mapping) is reached; the re-projection then leaves the
// expired rules out of the data plane (docs/user/firewall/rule-expiry.md). Without the agent's resync hook
// (Env.Resync nil: unit tests of other families) nothing is started.

import "ngfw/agent/internal/subsystems/ruleexpiry"

func (w *Wiring) registerRuleExpiry() {
	if w.env.Resync == nil {
		return
	}
	x := ruleexpiry.Start(w.RequestResync, w.env.Log.With("feature", "rule-expiry"))
	w.OnClose(x.Stop)
}
