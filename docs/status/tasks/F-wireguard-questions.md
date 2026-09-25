# F-wireguard — questions for the manager (written and kept going; none blocks the task)

## Q1 — Secret channel (PENDING-secret-channel): the end-to-end secret step stays open
Nothing carries key material from the API to the agent. What this branch does meanwhile:
- the agent has a per-family `subsystems.WireguardSecrets` store (D-051 `key/<n>`, `psk/<n>` → material), the
  `vpn.Resolver` of DF-5's descriptors and the builder's `key/…` → `x25519:<pub>` / `psk/…` → `hmac:<hex>` mapping;
- the product agent leaves it **empty**: DryRun reports `agent.secret-unavailable` (WARNING) at `privateKeyRef` /
  `presharedKeyRef`, and the apply fails loudly at that object (`wireguard.UnavailableRef` marker refused by Create,
  "…PENDING-secret-channel"); a peer is never created without its PSK;
- tests fill the store directly (unit, host checks: the stand-in for option 1's sealed cache, loaded before the first
  transaction); test builds only (`-tags vrxtestsecrets`) can load a slot-local 0600 fixture file (`VRX_TEST_WG_SECRETS`,
  `subsystems/wireguard_fixture.go`) — never compiled into a product build.
When the channel lands (option 1 recommended there), its receiver calls `WireguardSecrets.Put` (or replaces the store);
nothing else changes. The API's key-pair action stores the private key through the secrets service already.

## Q2 — `routeAllowedIps` default off (prompt open question) — decided: default false, flagged
TNSR does not auto-route allowed IPs. VPP 26.06 WireGuard interfaces are **NBMA**: a connected/attached route through
`wg<N>` is a drop, and the adjacency binds to the peer whose allowed IP covers the **next hop**
(`wireguard_if.c wg_if_update_adj`, `fib_path.c fib_path_attached_get_adj`). So without the flag (or a static route
`<prefix> via <addr in the allowed IP> wg<N>`) even the tunnel subnet is not reachable — proven on the host: the
kernel-peer ping failed without it and passed with it. The automatic routes use the prefix's first address as next hop
(the second for 0/0 and ::/0). Options: (a) keep false and document the static-route recipe (done in the user doc);
(b) default true for new interfaces in the UI form only; (c) default true in the schema (reshape of a default → PENDING).
I recommend (b) later (UI polish), keeping the schema default false.

## Q3 — DF-5 Q3 (src_ip dependency) — default (a) kept
No dependency on the address carrying `listenAddress`: the host check and the kernel-peer handshake passed with the
address configured outside the transaction (and with it not configured at all), so ordering never failed.

## Q4 — Peer-event watcher start: one line in `Wiring.Connected` (no anchor there)
TD-8's seams give an event sink (`Env.Publish`, used) but no "(re)connect" hook for a feature's subscription; the
only lifecycle seam, `DynamicSource.Run`, needs a descriptor instance of its own and desired KVs and starts only once
(the watcher must re-subscribe on every VPP connect: VPP drops the registration with the client). I used P08's
`Connected` hook with one line `w.wireguardConnected(ctx)` (subsystems.go, after DF-8's `Reconnected()`), exactly as
F-neighbors-ra does — expect a trivial union at merge. Without an event sink (unit tests) nothing is started. A
`Wiring.OnConnect(func(ctx))` seam (TD-8b?) would remove these lines from subsystems.go.

## Q5 — Names, descriptions and references Retrieve cannot get from VPP: agent-local `wireguard.meta`
VPP holds neither the configuration key of a WireGuard interface (it is `wg<instance>`, D-069) nor peer names,
descriptions or the D-051 reference a key came from, and `assemble()` gets no stored vpn document. I added an agent-local
descriptor `wireguard.meta` (file store `<state dir>/wireguard-meta-<owner>.json`, D-073b style, like F-object-model's
agent-local family): it is part of every transaction (journaled, rolled back, never written by DryRun) and the assembler
joins it with the VPP objects that exist. Without it Retrieve could never equal desired. OK?

## Q6 — Cross-domain objects of `vpn.wireguard`
`enabled`/`mtu`/`vrf`/`address[]` are DF-1/core objects of the `interfaces` domain on `interface/wg<N>`, and the
allowed-IP routes are `ip.route` objects of `routing`. They are projected only when the transaction includes those
domains (the API always sends all implemented domains); otherwise DryRun warns `agent.cross-domain`. The assembler moves
them back under `vpn.wireguard` and out of `interfaces` / `routing.static`. A direct gRPC Apply naming only `interfaces`
would delete them until the next full apply — acceptable?

## Q7 — `vpn.ipsec` / `vpn.pki` / `vpn.remoteAccess` while `vpn` is an implemented domain
Registering `Domains["vpn"]` makes the whole domain implemented. Until P11 / F-pki / F-ra-vpn land, the WireGuard
builder reports those subtrees as `agent.unsupported-field` (like P08's routing protocols), so `/state/drift` skips
them. Each of those rows removes its own pointer from that list in `desired/wireguard.go` (merge note).

## Q8 — DF-5's `vpn.Require` has no TD-11b ownership declaration (read-only package)
The non-owner `wireguard.async-mode` requirement (`vpn.Require`) declares neither `RecordsNoOwnership` nor
`CheckPersistent`, so TD-11b's guard would refuse a slot agent. I wrap it in `subsystems/wireguard.go` (`wgRegistry`,
`RecordsNoOwnership`); P11's `ipsec`/`ikev2` globals have the same gap. Better fix: one method on `vpn.Require` (P11 or the
manager, `descriptors/vpn` is read-only for this row).

## Q9 — VPP gaps (A7 V-new): per-peer counters and handshake time
The VPP 26.06 WireGuard API has no per-peer rx/tx counters and no last-handshake timestamp (`wireguard_peers_details`
carries flags and the endpoint only). WireguardState reports the interface's counters and the time the agent last saw
the peer become established (event). Filed as `### V-new (F-wireguard)` in docs/vpp-code-track.md.

## Q10 — Hostname endpoints
The schema allows a hostname as peer endpoint; this agent build does not resolve names (no DNS in the agent's apply
path): the projection fails with `agent.unsupported-value` at `endpoint/address`. A resolver belongs to F-object-model's
FQDN machinery (a later row).

## Q11 — TD-11b PartialCreate (manager addendum, D-133) — DONE
`peer.go` Create wraps the `want_wireguard_peer_events` failure (the peer already exists in VPP) in
`scheduler.PartialCreate`; `TestPeerCreatePartialWhenEventRegistrationFails` runs it through the reconciler: the
transaction fails, the peer is journaled and the rollback deletes it. On the old code the test fails ("peer result is not
a partial Create"; the peer stays in VPP).
