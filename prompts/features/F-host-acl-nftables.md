# Task: F-host-acl-nftables — local-in / management-plane ACL + nftables host-policy renderer   (prepend 00-CONTEXT.md)

## Goal
Protect the appliance's own host stack (SSH, HTTPS UI/API, SNMP, BGP/OSPF sessions punted to Linux, DNS/NTP served by the box)
with host ACLs rendered to **nftables** by a new agent renderer. This renderer is the single source of the host firewall
(D-057): F-hardening-lite consumes it, and the static base policy P10 ships in `vrx-meta` becomes its bootstrap default.
Reference: TNSR "host ACLs"; WBS D5.3 in `plan/wbs.csv`.

## Inputs to read first
- `packages/schema/src/domains/acl.ts`: `acl.host.<name>{rules[]}` (rule: `sequence, enabled, action accept|drop|reject, ipVersion,
  source, destination, service, interface?: linuxInterfaceName, log`) and `acl.hostAttachments[]{list, chain input|output|forward,
  priority −500…500, enabled}`; semantic `acl.host-attachments`; `docs/contracts/schema-nat-objects-acl.md` rows 140–158
- `apps/agent/internal/objects` (F-object-model) — expansion of address/service objects and groups (nft sets are built from it)
- `apps/agent/internal/renderers/{renderer.go,README.md,ALLOWLIST.md,helpers_*.go}` and a merged renderer as the pattern
  (`renderers/chrony` for restart/convergence, `renderers/frr` for escaping + DryRun)
- `prompts/P10-packaging-deb.md` §5 (static nftables base policy in `vrx-meta`) and `prompts/features/F-hardening-lite.md` (consumer)
- `docs/lab/shared-host-rules.md` — the host is shared and reached over SSH on ens192: **never load a ruleset into the host's root
  netns in tests**

## Contract changes
Needed only if you find gaps (e.g. `acl.hostSettings{defaultInput: drop|accept, managementInterfaces[], allowIcmp}`): additive
on `contract/F-host-acl-nftables` (schema + proto + drift guard + `-contract.md`), manager told via questions file; continue.

## Scope — build exactly this
Files you own: `apps/agent/internal/renderers/nftables/**`, `docs/agent/renderers/nftables.md`, `apps/agent/internal/agent/project_host_acl_nftables*.go`,
`apps/api/src/features/host-acl-nftables/**`, `apps/web/src/domains/firewall/host-acl-nftables/**`, `apps/web/src/locales/*/host-acl-nftables.json`,
`docs/user/firewall/host-acl-nftables.md`, `test/topology/host-acl-nftables/**`. Shared files: one-line appends only (ALLOWLIST.md
rows for `/usr/sbin/nft`, app.module import, router/nav entry).
1. **Renderer** `renderers/nftables`: renders ONE table `table inet vrx` (never `flush ruleset`, never touches other tables — P10's
   base policy and foreign tables survive); chains per attachment (`type filter hook <chain> priority <n>`), named sets for expanded
   objects, `ct state established,related accept` first, an **anti-lockout rule** (management SSH/HTTPS from the configured source
   cannot be dropped by a commit — DryRun error otherwise), counters on every rule, `log prefix "vrx:<list>:<seq> "` when `log`.
   Validate with `nft -c -f <staged>`; apply atomically with one `nft -f` file (`add table` + `delete table` + full table body in one
   transaction); Retrieve = `nft -j list table inet vrx` normalised; restart safety = re-render on agent start.
2. **Test isolation**: product paths vs `TestPaths(slot)`; tests render `table inet vrx_<slot>` and apply only inside a slot netns
   created by a test-only harness (the frrtest pattern; `ip` stays test-only in ALLOWLIST) — never the host root netns.
3. **API**: config via pointer routes; `GET /api/v1/state/host-acl` (rendered rules + per-rule packet/byte counters from `nft -j`).
4. **UI**: Host ACL page — lists (rule grid reusing F-acl's grid component only through ui-kit, else a simple DataGrid), attachments,
   counters, anti-lockout banner; en + fa.
5. **Docs**: `docs/user/firewall/host-acl-nftables.md` (default policy, anti-lockout, example "SSH only from 10.0.0.0/24") and the
   renderer mapping table in `docs/agent/renderers/nftables.md`.

## Acceptance (paste the evidence)
- [ ] Golden + hostile-input tests (quote/brace/newline injection in descriptions and interface names) green
- [ ] In the slot netns: after apply `nft list table inet vrx_<slot>` shows the rules (pasted); a blocked port from a veth peer is
      dropped and the counter increments; an allowed port connects
- [ ] Agent-restart simulation → table re-rendered identically (diff empty)
- [ ] Rollback → table content equals the previous revision (Retrieve, not assumption); other tables untouched (`nft list tables` before/after)
- [ ] A rule set that would drop the management SSH source → 400 problem+json with `pointer`; `tools/ci.sh --base main` green; UI screenshot

## Out of scope (do not build)
VPP data-plane ACLs (F-acl); objects CRUD (F-object-model); CIS/systemd hardening, package signing and shipping the static base policy
(F-hardening-lite, P10); punt/policer of control traffic inside VPP (not planned); NAT on the host; loading anything into the shared
host's root netns.

## Open questions to surface, not to decide silently
Which interfaces count as "management" for the anti-lockout rule before the NIC → port-group mapping exists (D-026)? Default: the
interface carrying the API's listen address. Should `reject` use icmpx admin-prohibited or tcp reset? Default: nft `reject` defaults.
