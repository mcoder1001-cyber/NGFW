# F-host-acl-nftables — questions for the manager

Written while working; none of these blocks the task (each states the default taken).

## Q1 — `AclConfig` field number 8 for `host_settings` (contract)
The envelope says a DesiredState field number for a config gap needs the manager first; wave-A-hotspots §2 *proposes*
"`AclConfig` (max 6) … 8 F-host-acl-nftables host settings". **Taken: 8** (`HostAclSettings host_settings = 8`, commit
`contract(proto): host acl state`). If you allocate another number, it is a one-line proto change + `pnpm gen`.

## Q2 — Decision: how the agent runs the renderer (prompt "Inputs", envelope "decision you must log")
- (a) one singleton scheduler descriptor `host-acl.nftables/vrx` wrapping the renderer, registered under `Domains["acl"]`
  (Create/Update = Render → `nft -c` → `nft -f`, Delete = remove the table, Retrieve = normalised `nft -j list table`)
  — no change to the agent core;
- (b) a renderer step in `agent/service.go` that P11/P12 reuse — the core is read-only in wave A (A5).
**Taken: (a).** Recorded in `docs/status/tasks/F-host-acl-nftables.md` → Decisions. For P11/P12/F-kea/F-unbound: (a) works
for any renderer whose actual state can be read back; the value carries the applied configuration (from an agent-local store)
plus the rendered model (from the daemon), see `docs/agent/renderers/nftables.md` "Descriptor".

## Q3 — Which interfaces are "management" for the anti-lockout rule (prompt open question, D-026)
The agent does not know the API's listen address, and the NIC → port-group mapping does not exist yet. **Default taken:** the
new setting `acl.hostSettings.antiLockout.interfaces` (empty = any interface) with `sources` (empty = any source) and `ports`
(default 22, 443). So out of the box the anti-lockout rule accepts TCP 22/443 from anywhere on any interface — the pfSense
behaviour; an operator narrows it by setting sources/interfaces. Alternative for later: default `interfaces` to the interface
carrying the API's listen address once the API passes it (e.g. a `management.listen` leaf).

## Q4 — `reject`: icmpx admin-prohibited or tcp reset? (prompt open question)
**Default taken:** nftables' own `reject` default (inet family: `icmpx port-unreachable`; nft prints it back as `reject`).
No setting added.

## Q5 — Product mode on this shared host (safety)
The product agent (owner `vrx`) renders `table inet vrx` into the **root** network namespace (mode `apply`) — that is the
product. On this dev host the product stack (`tools/app`, ports 3000/8080/9101) would therefore load a host firewall into the
root netns as soon as somebody commits `acl.hostAttachments` through the product UI. Nothing is loaded while no host list is
attached (no table at all), and the anti-lockout rule keeps TCP 22/443 open, but the product UI port here is 3000/8080, not 443.
**Proposal:** `tools/app` (manager-owned) exports `VRX_HOST_ACL_MODE=check` on this host (validate with `nft -c`, never load).
The worker never runs the product stack.

## Q6 — FQDN changes do not re-render yet
Host rules may use FQDN address objects; the renderer expands them with the objects runtime's current answers. A change of
the answers is picked up at the next Apply or resync, because `subsystems.Env.Resync` is not wired by `agent.go` yet
(`Wiring.RequestResync` is a no-op). When the A5 hook is set, `subsystems/host_acl.go` subscribes to the resolver and calls it.
