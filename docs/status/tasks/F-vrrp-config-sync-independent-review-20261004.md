# Independent HA source review

Initial frozen source `53a16f5670809e0cf00a44aa1b2df87b51ca1df8`, tree
`a008be84c07c76dc580da650732a064006fd6cbd`, against baseline `4c8d1b247`.
Reviewer edits only this report in Packaging-complete; HA source remains owned
by its developer. Scope auth/TLS/HMAC, atomic cluster commits, secret/local
exclusions, runtime state and propagation-loop suppression.

Initial verdict: **REQUEST CHANGES** for actual stale-revision acceptance.
`ClusterSyncService.receive` checks valid HMAC, timestamp and unique nonce but
does not reject an earlier source revision after a newer revision was committed.
The origin revision is only used in a comment. A timed-out delivery can arrive
after the next sync within30seconds and silently replace newer replica state
with old configuration. Fresh nonce prevents the existing replay guard from
detecting it; serialized source sending does not order delayed network arrival.
Developer independently confirmed the defect and is adding durable per-origin
source revision rejection inside the existing cross-process commit lock.

Other examined boundaries: pinned HTTPS sends no request/body bytes before leaf
pin and validity checks; no redirects/plain HTTP, bounded response, transport
abort. HMAC signature exact hexadecimal length and timing-safe comparison.
Public receiver additionally enforces trusted-proxy-aware requestProtocol,
known peer membership, bounded timestamp and replay cache; other endpoints
retain authenticated route guard. Cluster callback rechecks membership under
normal userSection/Pg advisory locking and refuses pending/inflight/dirty or
locked local candidates. Normal applyDocument performs existing full validation,
sealed-secret delivery, rollback and revision/audit behavior.

Synchronization excludes node-local cluster identity, system hostname/setup,
management/users, host/dataplane; explicit exclusions retain local values.
Credential values are redacted and only available local secret references can
apply. Automatic broadcast only confirmed in-sync master state, cluster-sync
revision suppression prevents ping-pong, manual force remains authenticated.
These source findings are not real multi-VM failover/packet acceptance.

Correction verdict and frozen SHA will be appended only after independent
review of the durable revision guard and actual focused regression evidence.

## Corrected final source

**APPROVE bounded HA source integration** at frozen
`a9f8f24dc525896f34e2e086bbe21c1da451c6ab`, exact tree
`7b86a5862b336008ffefe7e7e523231f3074053c`. Original request-changes finding
above remains preserved. No HA product edits performed by this reviewer.

Confirmed correction: receiver queries existing durable configuration revisions
of kind cluster-sync and exact origin comment prefix inside normal cross-process
commit locking. SQL left(comment,prefix.length)=prefix avoids LIKE wildcard
ambiguity for permitted peer names. The newest accepted source stamp must parse
as an exact safe integer; older/equal or malformed provenance refuses before
normal apply. Stamp and document use the ordinary atomic revision promotion.
This retains stale/replay protection across API restart without adding a second
state store. Deliberate sender database reset recovery is documented.

Additional developer self-audit, confirmed by reviewer: any-one-Master automatic
authority could permit both nodes to broadcast when virtual-router roles are
mixed, outside the declared multi-master merge scope. Final source requires
nonempty enabled configured router names, matching live rows for all those
names, no errors, and uniformly Master role before automatic sync. Disabled
routers do not participate; missing, mixed, unknown or errored state refuses.
Returned peer role is also unknown for nonuniform/unavailable observations.
Authenticated manual force still explicitly chooses the authoritative sender.

Independent actual focused execution on corrected final source:
`pnpm --filter @ngfw/api exec vitest run
src/features/vrrp-config-sync/service.test.ts --maxWorkers=1`:
**11/11 PASS**, tests697ms,total16.10s. Includes source5/equal6 refusal against
durablefloor6, newer7 acceptance; mixed/unknown/error/missing role refusals and
disabled-instance exemption, authentication/replay/local exclusion, loop and
shutdown cases. Earlier durablefloor-only freeze0aed3f2 independently passed
the same suite6/6. These use harmless DB/agent stubs and do not establish actual
PostgreSQL/multi-VM/VRRP failover or packet acceptance.

Reviewed the new VRRP owner-scoped runtime and subscription/polling paths,
existing engine gates and keepalived observed dump requirement. Manager must
preserve the final Event17 API relay mapping and both new watcher goroutines
when composing with newly merged native-SA source. Verify the final integrated
tree and applicable focused checks; owner full-CI waiver is not a lab PASS.

## API-managed coverage correction

**APPROVE** additional bounded delta `ee3bff260d09694651f84a4e4bdb90a77566a991`,
tree `59e3c96bc71c153df08c01a3650799c19e240842`, descendant of approveda9f8.
Independent five-file diff inspection confirms no earlier transport/lock/revision
guard changes. Configuration sync is no longer falsely classified as an unbuilt
agent engine. Enabled NAT/IPsec/ACL state replication still emits exact unsupported
leaf warnings, while disabled leaves do not claim an implementation deficit.
API-applied classification and VPP drift omission are restricted to /ha/cluster;
actual /ha/vrrp drift remains visible and unsupported coverage notes survive.
Developer evidence classification2+existingdrift4 PASS examined; no broad suite
rerun needed for this narrow classification correction. Final manager composition
must preserve all previously reviewed product code and route/event mappings.
