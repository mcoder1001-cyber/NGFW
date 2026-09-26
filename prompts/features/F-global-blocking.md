# Task: F-global-blocking — Global Blocking: block every IP in an uploaded list file   (prepend 00-CONTEXT.md)

## Goal
The operator uploads a text file of IP addresses and the box drops all traffic from and to them on every
interface, before the normal ACL policy. **IPs only for now** (IPv4/IPv6 addresses and CIDR prefixes). Domains,
URL feeds and scheduled downloads are out of scope and stay in `plan/backlog.yaml` (BL-SEC-03).

## Inputs to read first
- `packages/schema/src/domains/objects.ts`, `packages/schema/src/domains/acl.ts`, `semantic/acl.ts` — address objects and ACL attachments.
- `apps/agent/internal/descriptors/acl/**`, `docs/agent/descriptors/acl.md` — how ACLs reach VPP; hit counters.
- `apps/agent/internal/renderers/nftables/**` — host (local-in) policy, for traffic to the box itself.
- `apps/api/src/secrets/**` or any existing upload path — how large payloads enter the datastore.

## Contract changes
New schema domain path `/security/globalBlocking` (or under `objects` if the reviewer prefers — surface, don't decide):
`{ enabled, lists: [{ name, description, direction: both|inbound|outbound, entries: ip-prefix[] (≤ 200k total), action: drop, log }] }`.
Proto and api-client regenerate with `pnpm gen`.

## Scope — build exactly this
1. **Schema + semantics**: parse IPv4/IPv6 addresses and prefixes, normalise (host bits, duplicates, overlaps collapsed),
   reject bad lines with a pointer to the line number. Totals capped (≤ 200k).
2. **Import API**: `POST /api/v1/security/global-blocking/lists/{name}/import` takes a plain-text file (one entry per line,
   `#` comments and blank lines ignored), returns a preview (added / removed / invalid lines) and stages it into the candidate
   config; the normal commit applies it. Export returns the same format.
3. **Agent**: render each enabled list as a deny rule set applied first on the input side of every L3 interface (and output for
   `outbound`/`both`), ahead of the user ACLs; mirror it into an nftables set for local-in. Per-list hit counters.
   Apply must be incremental (diff of entries), not a full rebuild, and must stay atomic across commits.
4. **UI**: "Global Blocking" page — lists, upload with preview, entry count, hit counters, enable/disable.
5. **Tests**: parser/normaliser unit tests, semantic tests, descriptor tests with fake VPP, a topology test on the af_packet rig
   (blocked source dropped both ways, unblocked source passes, local-in blocked too), and a 200k-entry apply timing test.
6. **Docs**: `docs/user/security/global-blocking.md`.

## Acceptance (paste the evidence)
- [ ] Upload a file with mixed valid/invalid lines → preview shows the invalid line numbers; commit applies only after confirm
- [ ] Topology evidence: ping/TCP from a listed IP dropped through the box and to the box; counters increase (pasted)
- [ ] 200k-entry list commits in a measured time (pasted) and a 1-line change applies incrementally
- [ ] Screenshot of the Global Blocking page against the real endpoint
- [ ] `tools/ci.sh --base main` green

## Out of scope (do not build)
Domain/FQDN blocking, remote feed URLs and scheduled refresh, Geo-IP, DNS sinkhole (all in `plan/backlog.yaml`).

## Open questions to surface, not to decide silently
- VPP mechanism for large lists: ACL plugin rules vs classify tables vs a dedicated feature — measure lookup cost at 200k entries.
- Where the domain lives (`security` new domain vs `objects`) and whether it needs its own RBAC permission.
