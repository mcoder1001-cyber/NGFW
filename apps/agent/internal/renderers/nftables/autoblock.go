package nftables

import (
	"crypto/sha256"
	"fmt"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

func scanEnabled(cfg *ngfwv1.AutoBlock) bool {
	if !cfg.GetEnabled() {
		return false
	}
	for _, r := range cfg.GetRules() {
		if r.GetSource() == "portScan" && r.GetEnabled() {
			return true
		}
	}
	return false
}

// A separate input hook observes distinct destination ports after runtime blocks
// but before user local-in policy. Accept terminates this observer only; later
// base chains still enforce the host policy. Bounded logs protect the journal.
func scanChain() *Chain {
	c := &Chain{Name: "in__scan", Hook: "input", Priority: -299, Policy: "accept"}
	for i, text := range []string{
		`tcp flags & (fin | syn | rst | ack) == syn limit rate 100/second burst 200 packets counter log prefix "ngfw:scan " accept`,
		`meta l4proto udp limit rate 100/second burst 200 packets counter log prefix "ngfw:scan " accept`,
	} {
		h := sha256.Sum256([]byte(text))
		c.Rules = append(c.Rules, &Rule{Comment: fmt.Sprintf("ngfw:@scan/%d:%x", i, h[:4]), Text: text, Kind: "port-scan", Pointer: "/security/autoBlock", Verdict: "accept"})
	}
	return c
}
