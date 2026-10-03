# Combined source integration review

Immutable merge source `53165b5d397b49d73906d488921086c064e10555`.
Verdict: APPROVE SOURCE INTEGRATION WITH LIMITS; no new blocking wiring collision.

app.module.ts imports notificationsFeature and pkiFeature once each and spreads
each controller/provider array once; their feature entries are distinct classes.
PKI's agent-files provider uses the typed pkiFileState method and owner/deadline
on its own read-only lazy channel, avoiding changes to the shared agent client.

Management retains tags 1–6 and notifications occupies new tag 7. Notification
messages do not duplicate PKI names or RPCs; PKI config additions use distinct
existing-message tags and the PkiFileState RPC is present in Dataplane. API desired
state conversion removes API-owned notifications before passing state to the agent.

VPN tabs preserve srv6/lisp/wireguard order and append pki once. The cross-namespace
label pkiInventory:title agrees with centrally preloaded NAMESPACES and both resource
maps/imports. The lazy component export and corrected public state GET agree with
the mounted panel source; locale registration no longer depends on panel import.

No product writes or tests. Go generated bindings were being regenerated separately;
this report approves source composition, not pending generated outputs or CI. Actual
agent PKI materialization/RPC availability is a separate deferred backend boundary,
not proven here; the panel uses an unavailable warning rather than secret output.
