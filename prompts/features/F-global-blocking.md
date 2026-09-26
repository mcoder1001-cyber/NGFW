# Task: F-global-blocking — Global Blocking: block the IPs of a list file (uploaded or downloaded from a server) on chosen interfaces   (prepend 00-CONTEXT.md)

## Goal
The admin gives the box a text file of IP addresses, either by **uploading** it or by entering a **server URL** the box
downloads it from, and chooses **which interfaces** the list is enforced on. The box drops traffic from and to those IPs on
the chosen interfaces, before the normal ACL policy. **IPs only for now** (IPv4/IPv6 addresses and CIDR prefixes).
Domain lists and DNS sinkhole stay in `plan/backlog.yaml` (BL-SEC-03).

## Inputs to read first
- `packages/schema/src/domains/objects.ts`, `packages/schema/src/domains/acl.ts`, `semantic/acl.ts` — address objects and ACL attachments.
- `apps/agent/internal/descriptors/acl/**`, `docs/agent/descriptors/acl.md` — how ACLs reach VPP; hit counters.
- `apps/agent/internal/renderers/nftables/**` — host (local-in) policy, for traffic to the box itself.
- `apps/api/src/secrets/**` or any existing upload path — how large payloads enter the datastore.

## Contract changes
New schema domain path `/security/globalBlocking` (or under `objects` if the reviewer prefers — surface, don't decide):
`{ enabled, lists: [{ name, description, source: upload|url, url?, refresh? (interval, e.g. 1h; none = manual),
tls? { verify: true, caRef? }, auth? { secretRef } , interfaces: interface-ref[] (≥ 1, or "all"), direction: both|inbound|outbound,
entries: ip-prefix[] (≤ 200k total), action: drop, log }] }`.
Credentials for the URL live in the secret store (secretRef), never in plain config.
Proto and api-client regenerate with `pnpm gen`.

## Scope — build exactly this
1. **Schema + semantics**: parse IPv4/IPv6 addresses and prefixes, normalise (host bits, duplicates, overlaps collapsed),
   reject bad lines with a pointer to the line number. Totals capped (≤ 200k).
2. **Import — two sources, one parser** (plain text, one entry per line, `#` comments and blank lines ignored):
   - **Upload**: `POST /api/v1/security/global-blocking/lists/{name}/import` takes the file, returns a preview
     (added / removed / invalid lines) and stages it into the candidate config; the normal commit applies it.
   - **URL**: the list stores an http(s) URL. "Fetch now" downloads it and shows the same preview. With `refresh` set, the
     box re-downloads on schedule and applies the diff automatically (audited as a system change). Rules: https by default
     with certificate verification, size cap (e.g. 20 MB), timeout, `If-Modified-Since`/ETag, and **on any failure (network,
     HTTP error, parse errors above a threshold, empty file) keep the last good list** and raise an alarm; never apply an
     empty or partial list. The management VRF/interface is used for the download.
   - Status per list: source, last fetch time, last result, entry count, next refresh. Export returns the same text format.
3. **Agent**: render each enabled list as a deny rule set applied first on the input side of **the interfaces the list selects**
   (and output for `outbound`/`both`), ahead of the user ACLs; a list may select "all" (current and future L3 interfaces).
   Selecting a deleted interface is a validation error, as for ACL attachments. Mirror it into an nftables set for local-in
   when a selected interface carries traffic to the box. Per-list hit counters.
   Apply must be incremental (diff of entries), not a full rebuild, and must stay atomic across commits.
4. **UI**: "Global Blocking" page — lists; per list: source (upload file / server URL + refresh interval + "Fetch now"),
   preview before apply, interface multi-select (or "all"), direction, entry count, last fetch status, hit counters per
   interface, enable/disable.
5. **Tests**: parser/normaliser unit tests, semantic tests, descriptor tests with fake VPP, a topology test on the af_packet rig
   (blocked source dropped both ways on a selected interface, **passes on an unselected one**, unblocked source passes,
   local-in blocked too), a fetcher test against a local http server (success, 404, timeout, oversize, garbage → last good
   list kept), and a 200k-entry apply timing test.
6. **Docs**: `docs/user/security/global-blocking.md`.

## Acceptance (paste the evidence)
- [ ] Upload a file with mixed valid/invalid lines → preview shows the invalid line numbers; commit applies only after confirm
- [ ] URL source: fetch from a test server, change the file, scheduled refresh applies the diff; stop the server → last good list kept, alarm raised (pasted)
- [ ] Same list enforced on interface A and not on B (pasted)
- [ ] Topology evidence: ping/TCP from a listed IP dropped through the box and to the box; counters increase (pasted)
- [ ] 200k-entry list commits in a measured time (pasted) and a 1-line change applies incrementally
- [ ] Screenshot of the Global Blocking page against the real endpoint
- [ ] `tools/ci.sh --base main` green

## Out of scope (do not build)
Domain/FQDN blocking, Geo-IP, DNS sinkhole, curated feed catalogue (all in `plan/backlog.yaml`).

## Open questions to surface, not to decide silently
- VPP mechanism for large lists: ACL plugin rules vs classify tables vs a dedicated feature — measure lookup cost at 200k entries.
- Where the domain lives (`security` new domain vs `objects`) and whether it needs its own RBAC permission.
