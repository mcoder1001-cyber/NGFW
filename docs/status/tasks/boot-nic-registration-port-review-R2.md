# Review R2 (security) — boot-nic-registration-port

Reviewed head `814532721` (base `origin/main` `99a774734`), PR #238. Reviewer: panel R2 (Opus 5.5, read-only agent).
The reviewer handed its report back in chat and did not write this file itself (read-only run); the developer saved it
here unedited. Developer answers follow at the end.

## Verdict: APPROVE WITH CHANGES (0 BLOCKER, 0 MAJOR, 3 MINOR, 3 NIT)

**Summary.** The port matches `76484d79f` and drops no safeguard. Agent inventory data cannot reach the stored document
unvalidated: names are checked by `logicalIfName.safeParse`, and PCI strings by Go `PCIAddress()` and again by the schema
`pciAddress` at commit. The audit payload carries only names, PCI addresses and reason-only management notes. There are no
shell calls, routes, dependencies or secrets in the diff. Released rows and saved `managementPci` are protected in
`seedDocument`. Registration only writes configuration; binding a NIC needs the separate, approved, management-protecting
startup apply.

### 1. MINOR — rollback to before a NIC's registration removes its built-in row; the next boot hands a released NIC back to the engine; guard comments out of date
Files: `apps/api/src/datastore/physical-nics.ts:6-9`, `:31-35`, `:82`; `seed.service.ts:301-320`;
`docs/user/interfaces/default-dataplane-nics.md:41` vs `:98`. Under D-249 markers are created at any revision N; a rollback
(`POST /config/rollback/:rev`) to a revision older than N removes those rows without `assertPhysicalNicEdit`; `clusterCommit`
can do the same (HA sync copies `/interfaces`, `/dataplane` stays node-local — pre-existing). The operator releases ens161,
rolls back to before registration, and the next API boot registers ens161 again as `owner: dataplane`. Fix: update the
comments and the 403 message; document the lost release; preferably have rollback return a warning listing dropped rows.

### 2. MINOR (correctness part [other: R1]) — a persistent commit failure loops every 60 s forever; a concrete trigger
Files: `seed.service.ts:179-182`, `:96-107`, `:325-331`; `packages/schema/src/semantic/dataplane.ts:91-114`
(`dataplane.management-not-dpdk`); `packages/schema/src/domains/dataplane.ts:111` (`managementPci` max 4). Boot 1 registers
ens161 engine-owned; boot 2 the host flags ens161 as management (new default route) and a new NIC appears → `seedDocument`
appends `0000:04:00.0` to `managementPci` while it is whitelisted → semantic validation fails → treated as `deferred`, retried
every 60 s for the life of the process; the new NIC is never registered; no event. Fix: do not append a management PCI that
is in `pciWhitelist`/`devices` (record a warning naming the row); treat a 4xx ProblemError as final for this boot with one
warning event, or cap the retries.

### 3. MINOR — every-boot registration widens the management-misdetection window on installs never seeded
Files: `seed.service.ts:127-136`, `:147`. Without a saved `managementPci`, protection depends only on what the host flags at
that boot (default route, SSH peer, `NGFW_MGMT_*`). Mitigations: generator/apply-startup reject management NICs; approval;
dead-man rollback. Fix: record a warning naming the NIC(s) taken as management, or require a stable source.

### 4. NIT — SSH peer address can reach the API journal through the inventory error message (pre-existing)
`hostfacts.go:299,312`, `rpc_host_nics.go:62-65`, `agent.client.ts:750`, `seed.service.ts:138-141`: "unresolved br0 (control
connection from <peer>)" logged at warn. Fix: pass the message through `reasonOnly()`.

### 5. NIT — adoption through `devices.<pci>.name` lets an operator-role user mark an existing non-physical row as built-in
`RESERVED_NAME` covers only `local0|loopN|host-` while the generator reserves many more stems (`vppstartup/validate.go:50`).
Fix: align with `reservedStems`; do not adopt rows with type-specific fields (bond, pppoe, …).

### 6. NIT — `systemCommit` ignores the configure lock (`lockedAt`) while `clusterCommit` refuses (pre-existing)
`commit.service.ts:609-633` vs `:643`. Consider deferring when `c.lockedAt !== null`.

## Evidence
```
$ tools/ci.sh check --base origin/main
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no secret-shaped strings
ok: gitleaks — scanned ~37368 bytes (37.37 KB) in 651ms no leaks found
check PASSED (0m10s)
$ gitleaks detect --no-git -s <scratch copy of the 17 changed files>   → no leaks found
$ git diff origin/main...HEAD | grep -nE '(BEGIN .* PRIVATE|password\s*[:=]|token\s*[:=]|postgres(ql)?://[^/ ]*@|172\.30\.|ghp_|github_pat_|AKIA)'  → (none)
$ git diff origin/main...HEAD | grep -nE '(child_process|execSync|spawn\(|exec\(|sh -c|vppctl|cli_inband|eval\()'  → (none)
$ git diff origin/main...HEAD -- '**/package.json' pnpm-lock.yaml go.mod go.sum  → (none)
$ tsx probe (real seedDocument + RootConfig + validateSemantics):
boot1 registration: valid
boot2 registration: SEMANTIC FAIL: /dataplane/managementPci/1: management NIC 0000:04:00.0 must never be a DPDK device
five management NICs: SCHEMA FAIL: dataplane/managementPci: Too big: expected array to have <=4 items
after rollback to pre-registration revision, next boot: {"enabled":true,"physical":{"pci":"0000:04:00.0","owner":"dataplane","builtIn":true}}
constructor own key: true Object.prototype polluted: false
```

## Developer answers (second iteration, see the wip file for the SHA)
- #1: comments and 403 text updated in `physical-nics.ts`; user doc documents the lost release and the HA same-layout
  requirement; rollback warning → `docs/tech-debt.md` row (commit-engine change, out of this port's scope).
- #2: fixed — a management PCI in `pciWhitelist`/`devices` is never appended and is reported as `management-engine-owned`
  (audit payload + `system.seed-defaults-skipped`); `managementPci` capped at 4 (`management-limit`); a 4xx ProblemError is
  final for the boot with one `system.seed-defaults-failed` event; other failures back off exponentially (1 min → 1 h).
  Tests: `planRegistration` conflict/limit cases, final and transient failure cases.
- #3: the `system.seed-defaults` payload now lists the NICs taken as management (`management`), with the reason-only notes.
- #4: fixed — the unreachable/attempt-failed messages pass through `reasonOnly()`.
- #5: fixed — `isReservedIfName` mirrors `reservedNames` + `reservedStems`; rows with `bond` or `pppoe` are never adopted.
- #6: pre-existing (affects the block-list refresh too) → `docs/tech-debt.md` row; not changed in this branch.
