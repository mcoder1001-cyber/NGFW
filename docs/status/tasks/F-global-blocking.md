# F-global-blocking — IP block lists (cloud session charming-johnson, 2026-09-27)

Upload a file or give a server URL; the box drops the listed IPv4/IPv6 addresses and prefixes on the chosen
interfaces ahead of the access lists and, optionally, on traffic to the box itself. User page:
`docs/user/security/global-blocking.md`.

## Decisions (the prompt's open questions)
- **Q1 — where it lives:** `acl.globalBlocking` (same agent family and attachment semantics as `acl.lists`), not a new
  root domain; no new RBAC permission (config edits are operator, like ACLs). Proto `AclConfig.global_blocking = 9`.
- **Q2 — VPP mechanism:** ACL-plugin deny ACLs, hash-bucketed: per list and direction `nextPow2(ceil(n/4096))`
  buckets, ≤ 64 (`_gb.<list>.i<nn>` / `.o<nn>`), prepended to every selected interface's binding. The bucket count is
  stable while the list stays in the same power-of-two band, so a one-line change replaces one bucket ACL
  (`acl_add_replace` of ≤ ~4 k rules), no rebinding. The ACL plugin's TupleMerge lookup keeps the per-packet cost flat
  in the number of rules of one mask; the lab row measures it (below). Classify tables would need a new descriptor
  family; not needed for 200 k.
- **Implicit deny:** VPP drops what no ACL of a bound direction matches, so an interface with no user ACL in a direction
  gets `_gb.pass` (permit any v4 + v6) after the block lists. Never added where the user has an ACL (it would override
  the user's implicit deny).
- **protectHost** (explicit flag, default true): nftables sets `b4_/b6_<list>` and chain `in__gb` (input, priority
  −300, before conntrack; drops established traffic too; on every host interface — the VPP↔Linux interface mapping
  is not known to the renderer). Anti-lockout sources, when configured, are accepted first; with no sources they are
  not (the list would otherwise never block SSH/HTTPS).
- **Entries in the document** (≤ 200 k total, canonical, the API collapses overlaps; the agent/renderer tolerate
  overlaps). Large revisions are noted as tech debt.
- **Scheduled refresh = a system change:** `CommitService.systemCommit` applies running + the new entries as revision
  kind `system` with a null author, only when the candidate has no uncommitted edits and no confirm-timer commit is
  pending (else *deferred*, retried at the next check). It does not touch secret versions.

## Security review before merge (adversarial re-read of the diff)
- **Fixed — credential redirection:** secret references are admin-only (P06 §6), but an operator could re-point the
  `source.url` of a list whose admin-set `authRef` exists (or set `verifyTls: false`) and the box would send the
  token/password to that server on the next fetch. `privilegedChanges` now treats `source.url` / `source.verifyTls` of
  a list with an authRef as admin-only (edit and commit). e2e: 403 at `/acl/globalBlocking/lists/cred/source/url` and
  `…/verifyTls` for an operator; other fields still editable.
- **Fixed — reading internal HTTP through the preview:** a download's invalid lines are returned by line number and
  reason only (the upload preview still shows the text the user sent). e2e asserts `text: ''`.
- Kept by design: http:// URLs are allowed (schema) for internal feeds; no redirect is followed; downloads are capped
  (20 MB, 30 s) and never replace a list with an empty or mostly invalid file.

## Built
- Schema/semantics (31ce01a9): `global-blocking.ts`, the one parser `global-blocking-parse.ts` (normalise, dedupe,
  collapse, invalid lines with line numbers), `acl.global-blocking-interfaces-exist`.
- Agent (da466ca4): `desired/global_blocking.go`, `actions/acl` applied record (`acl.config/global-blocking`,
  `GlobalBlockingFingerprint`), `AssembleACL` keeps `_gb.*` out of `acl.lists`/`attachments` and reconstructs the block
  lists on drift; `renderers/nftables/blocking.go`.
- API (a38718bb): `/api/v1/security/global-blocking` status (source, last download, next refresh, data-plane and
  host drop counters), `lists/{name}/import` (text/plain ≤ 8 MB, preview by default), `lists/{name}/fetch`,
  `lists/{name}/export`; `fetch.ts` (https verify, private CA / token / basic from secret refs, 20 MB, 30 s, ETag /
  If-Modified-Since, no redirects); scheduled refresh (`NGFW_GLOBAL_BLOCKING_CHECK_SEC`, default 60 s; a Valkey
  `SET NX` keeps it to one API process per list); `BLOCKLIST_FETCH_FAILED` / `BLOCKLIST_REFRESHED` system events.
  ACL attachments view: `_gb.*` no longer counts as out of sync.
- Web: Firewall › Global blocking (`domains/security/global-blocking`): lists with source, enforcement, entries
  (candidate vs applied), last download, drops; add/edit (schema form, interface picker), import file / fetch now
  with a preview (invalid lines, added/removed samples) before staging, export, delete; en + fa.

## Evidence (this session)
- Parser: 200 000 lines parse in ~0.65 s (`packages/schema` test).
- Agent unit (`desired/global_blocking_test.go`): projection order (`_gb.bad.i00,a` / `_gb.bad.o00,_gb.pass`), rule
  shapes validate, assemble == configuration, drift reconstructs, allInterfaces / disabled / empty, 10 000 entries →
  4 buckets and a one-entry change touches ≤ 2 bucket ACLs, 200 000 entries project in **1.45 s** into 64 buckets
  per direction (65 ACLs per direction on the interface), name/tag and 255-ACL limits.
- Fake VPP end to end (`agent/rpc_global_blocking_test.go`): bindings in VPP `_gb.bad.i00,user-in,_gb.bad.o00,_gb.pass`
  (loop711) and `_gb.bad.i00,_gb.pass,_gb.bad.o00,_gb.pass` (loop712); Retrieve == desired; second Apply empty; one
  added entry = 3 updates (two buckets + the applied record), 0 creates/deletes, no rebinding; AclState lists the
  bucket ACLs; removing the lists restores `user-in` alone.
- **Real kernel** (nft 1.1.x in a slot netns, `NGFW_INTEGRATION=1 NGFW_TEST_PREFIX=w1`,
  `TestIntegrationBlockListProtectsHost`): the listed peer 10.1.77.2 is dropped (block rule counter 2 packets), an
  unlisted peer connects; **200 000-entry set applied in 6.3 s**, Retrieve 2.0 s and == desired; a one-entry change
  re-applies in 8.1 s (whole-table replacement, atomic); removal clean. The existing host-ACL integration test passes
  with the renderer changes.
- API: fetcher against a local server (200 + ETag, 304, 404, redirect refused, timeout, declared and streamed
  oversize, bad scheme, refused connection); e2e (PostgreSQL + Valkey + fake agent): upload preview with the invalid
  line number → nothing staged → stage → commit → export; status with hits; URL fetch now → commit; scheduled refresh
  not due / applied as revision kind `system` with null author (`1 added, 1 removed`); HTTP 500, >10 % garbage and an
  empty file each keep the last good list and raise `BLOCKLIST_FETCH_FAILED` ×3; an edited candidate defers.
- Web: jsdom page tests (list row, preview with the invalid line, staging `dryRun=false`; Persian RTL with the URL
  LTR; readonly role); full web suite 467/467; `check-logical-css` OK.
- Gates: `go test -race ./...` (agent) all ok; golangci-lint 0 issues; API unit 300/300, e2e 218 passed / 3 skipped;
  web typecheck/lint; `tools/ci.sh check --base origin/main` PASSED; `buf breaking` clean.
- Noted, not mine: `internal/renderers/rsyslog` `TestDescriptorDeferred` failed once under the full `-race` suite
  (`pending []`); 20/20 under `-race` alone and green on the full re-run — load-sensitive stand-in process timing.

## Not done here → `F-global-blocking-host` (lab VPP)
- Topology on the af_packet rig: a listed source dropped both ways on a selected interface, **passing on an unselected
  one**, an unlisted source passing, local-in blocked too, VPP counters increasing; `vppctl show acl-plugin acl` /
  `show acl-plugin interface` before/after; the lookup cost at 200 k entries (`show runtime` acl-plugin-in-ip4-fa
  clocks/packet with 0, 10 k, 200 k entries); 200 k commit time end to end; screenshot of the page against the real
  endpoint.
