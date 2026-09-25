# F-kea-dhcp-relay — questions for the manager (written and kept going; nothing waits on them)

**Q1 — anchors.** The addendum says "insert only under your `wave-BC: F-kea-dhcp-relay` anchors (seeded by W-seed-BC)". The
files carry `// wave-A: F-kea-dhcp-relay` anchors (W-seed seeded F-kea/F-unbound with wave A's follow-ons); W-seed-BC added
none for this task. All shared hunks sit directly below the `wave-A: F-kea-dhcp-relay` anchors (list in F-kea-dhcp-relay.md).

**Q2 — relay on an interface that also runs a Kea server in the same VRF (prompt's open question).** Allowed, and
proposed to stay allowed: it is a coherent design (VPP relays the LAN to the box's own Kea), and it is exactly the lab
topology — server `lan` names `host-w2l0`, relay `to-kea` relays `host-w2l0`'s VRF to the Kea instance. No rule refuses it.

**Q3 — P02c example vs the new "one Kea VRF per family" rule.** `packages/schema/examples/services-dhcp-dns.json` has two
enabled IPv4 servers in VRFs `default` and `customer-a`; the new semantic rule `services.kea-dhcp-relay-one-vrf-per-family`
(asked for by the prompt; RF-3's renderer refuses it at apply) makes that example semantically invalid. No test runs my
rule on it (the services example test runs the P02c group only), and the agent's projection deliberately does **not**
raise it (the renderer does, at apply) so `TestProjectSchemaExamples` stays green. Proposal: the example owner moves
`customer-a` to `enabled: false` or to `ipv6`; I did not edit it (not my file).

**Q4 — DHCPv6 client (IA_NA / PD).** The schema has no DHCPv6 client leaf (`interfaces.<n>.dhcpClient` is DHCPv4 only) and
no Interface field number is allocated to this task, so `dhcp.dhcp6-client` / `dhcp6-pd-client` / `dhcp6-pd-address` are
not projected (not registered either). When a leaf arrives, the UI shows "configured, state unknown" (the objects are
write-only, D-063; `show dhcp6 clients` is CLI-only) — the prompt's proposal, which I support.

**Q5 — defect found by code reading (P08 / core, not fixed here).** On an interface with `dhcpClient`, the DHCPv4 lease
becomes an address in VPP. `core/ifaddr.go` Retrieve reports every address of an owned interface, so the next
reconcile of `interfaces` deletes the leased address as "not desired" (and `/state/drift` would show it). Proposal: the
interface-ip Retrieve skips the address `dhcp_client_dump` reports as the lease of that interface (DF-8's
`ClientDescriptor.Leases` gives it). The topology test keeps VPP's client in DISCOVER on purpose (no server on its link),
so it never exercises that path. Owner: P08/core — a TD row?

**Q6 — Kea interfaces in the product.** Kea binds Linux interfaces; until linux-cp (P12) provides the VPP→Linux mapping,
the product agent refuses a server interface (RF-3 review L7, `NoMapper`) — so DHCP servers are committable in the
product only after P12. The product agent on this dev host reads `/etc/kea/kea-dhcp4.conf` (the packaged, commented file):
Retrieve treats it as foreign (no embedded input) — never reported, deleted or rewritten.

**Q7 — test mode in the product binary.** `VRX_KEA_MODE=test` makes the agent's Kea runner allow `/usr/bin/ip` (the
`ip netns exec <validated ns> kea-dhcp4 -t <staged file>` trampoline, fixed argv) — RF-3 kept that trampoline out of
`NewRunner`. It is reachable only through the agent's own environment (root). Alternative if you prefer: a `//go:build
labtest` file for the test mode.

**Q8 — `Domains["services"]` and the unsupported-field notes.** This task is the first lander of `Domains["services"]`
(const `Services` + the entry under my anchors). Other `services` features append their descriptor names to that entry
and add their sub-key to `desired.ServicesImplemented` (an `init()` in their own file) so the agent stops reporting
`/services/<key>` as `agent.unsupported-field`.

**Q9 — TD-11b Q3 (addendum).** Done inline in `descriptors/dhcp/client.go` (TD-11b's `ClaimFirst` helper is not on this
base); when TD-11b merges, the Create can switch to `tg.ClaimFirst(ctx)` / `Claim.Adopt` / `Claim.Undo` 1:1.

**Q10 — shared test files touched** (beyond the anchors): `agent/service_test.go` (canonical Retrieve doc gains
`"services": {"dhcp": {}}`; the two domain-list assertions compare with `implementedDomains()` — the same edit
F-object-model made), `descriptors/core/coretest/fakevpp.go` (one hook line, model in the new `coretest/kea_dhcp_relay.go`),
`apps/api/src/testing/fake-agent.ts` (one import line: it has no import anchor).

**Q11 — lease release in the lab.** `dhclient -sf /bin/true` leaves the client interface unconfigured, so a DHCPRELEASE
cannot be unicast; the test stops dhclient with `-x` and the lease expires in Kea (600 s). Harmless; noted so nobody
"fixes" it by running the real dhclient-script inside the netns (it rewrites the host's resolv.conf).
