# Review — F-sdk-terraform-ansible (branch `task/F-sdk-terraform-ansible`, HEAD 20c9bca)

Independent review agent. Run directly on the host, slot 5 (`w5`).

## What I ran
| check | result |
|---|---|
| `tools/ci.sh --base main` | first run **failed** in `apps/agent: make lint` with `parallel golangci-lint is running`. That was lock contention with another agent's run and has nothing to do with this branch. The re-run **PASSED** (wall time 3m28s, logs `/root/ngfw-wt/logs/ci/F-sdk-terraform-ansible-20260924-034431-2538904`). It matches the pasted output. |
| `sdk/test.sh` | 27 passed, 1 skipped (live) · golangci-lint `0 issues.` · `go test` ok (client, provider) |
| `sdk/gen.sh --check` | full API build → `gen: clean` |
| live `test/topology/sdk-terraform-ansible/live.sh run …` (Python live + `TestLive`) | both PASS against the real API, vrx-agent and VPP (`loop511`/`loop521`/`loop522` addresses seen in `vppctl`). Teardown was clean: 0 loops left in VPP, 0 `vrx:w5:*` keys, `vrx_w5` dropped, `/run/vrx-test/w5` holds only the pre-existing `df8-globals.lock`. |
| probes (a `go test -overlay` file in my scratchpad, plus a crafted OpenAPI fed to `gen.py` in the scratchpad; the tree was not modified) | they confirm H1, H2 and L1 below |

Checklist items that raised nothing:
- (1) Contract: no contract files changed.
- (2) Real verification: the live tests assert on `vppctl show int addr` and `/state/interfaces`.
- (3)/(4) Restart safety and VPP API provenance: not applicable (no agent or binapi changes).
- (5) Shared-host rules: `live.sh` stops by PID, binds the API to 127.0.0.1, uses slot-prefixed DB, Valkey keys and socket, and cleans up in `trap`/`t.Cleanup`.
- (8)/(10) UI and i18n: not applicable.
- (9) Scope: Ansible is cut per D-085/D-093. Nothing extra was built.
- Licences: the HashiCorp modules are MPL-2.0. That is acceptable for a separately distributed provider binary (D-093). The rest is MIT, BSD or Apache-2.0.
- Python test lock: hash-pinned. `--require-hashes --no-deps`, pure-python wheels, one sha256 per pin.

## Findings (by severity)

### H1 — `vrx_config` plan modifiers change planned values of non-computed attributes, which real Terraform rejects; the harness does not model that check
`sdk/terraform/internal/provider/config_resource.go:51-57` (`pointer` and `value` are `Required`, not `Computed`) together with
`semanticJSON` (`:276-292`) and `samePointer` (`:294-310`). Both set `resp.PlanValue = req.StateValue` whenever the config string
differs from the state string but means the same thing.

**Failure scenario.** Terraform core validates each plan (`objchange.AssertPlanValid` → `assertPlannedValueValid`): a
non-computed attribute must be planned *exactly* as configured. Otherwise the run aborts with *"Provider produced invalid
plan … planned value … for a non-computed attribute"*. This fires in ordinary use:
- after `terraform import`: state `value` is the canonical full node, and the user writes the same JSON with a different key order or whitespace;
- after drift has replaced state `value` with the live node, and the user then aligns the config to it;
- when the pointer is spelled `interfaces/loop1/` in state and `/interfaces/loop1` in config (or the reverse).

My probe (the provider's own fake API, harness plan, then comparing planned with config) reports:
`planned pointer="interfaces/loop1/" but config pointer="/interfaces/loop1"` and
`planned value="{\"enabled\":true,…}" but config value="{ \"ipv4\": …, \"enabled\": true }"`.
`provider_test.go:73` and `:91-95` assert the opposite and pass, because `tfharness.PlanChange` (`internal/tfharness/harness.go:115-158`)
never compares the planned state with the config. The "second plan: No changes" evidence therefore does not prove the
provider works under a real CLI in these paths. The rest of the evidence is labelled honestly as an emulation.

**Fix.**
- Delete `samePointer`. Instead, `ValidateConfig` should reject a non-canonical pointer (or store the pointer exactly as configured and compare normalized only in `Read`).
- Drop `semanticJSON` as a plan modifier. Either use a custom string type with semantic equality (`jsontypes.Normalized`, applied by the framework on Read/Apply), or accept that a reformatted config shows a harmless update: `Apply` already returns `unchanged` without a commit.
- Add core's plan-validity rule to `tfharness` (planned == config for every non-computed, non-write-only attribute; write-only attributes planned null) so the harness catches this class of bug.
- The real-CLI run (questions #7) is still required before release.

### H2 — the write-only guard runs only when `value` is known at validate time; `passwordHash` can land in the state file
`config_resource.go:91-103` checks `writeOnlyLeaves` only in `ValidateConfig` and skips unknown values. `body()` (`:113-131`, used by Create
and Update) never re-checks.

**Failure scenario.** `value = jsonencode([{ username = "ops", role = "operator", passwordHash = random_password.ops.bcrypt_hash }])`,
or any reference to another resource, is unknown during validation. The guard passes. Plan and apply then carry the known
value, the provider PUTs it and stores `plan.Value` in state.

Probe: ValidateResourceConfig with an unknown `value` gave 0 diagnostics. Plan and apply with the known value at
`/management/users/1` succeeded, and the new state held the hash (`hash in new state: true`). This violates the task's
acceptance item "a password hash never appears in plan output, state files" and D-046.

**Fix.**
- Call `writeOnlyLeaves` in `ModifyPlan` whenever `value` is known, and again in `body()` before any HTTP call. Return an error in both places.
- Add a test with an unknown value at validate time.
- Apply the same guard to `vrx_interface` once its schema gains secret members (today it has none).

### M1 — `partially-applied` / `not-applied` and `notApplied[]` are silently reported as success
`internal/client/client.go:294-303`: `CommitResult` has no `NotApplied` or `Sync` field. `:369-379`: any non-`pending` status is returned
as success, and after a confirmed commit only the confirm answer is kept. The API answers a confirm with `notApplied: []`
(`apps/api/src/commit/commit.service.ts` `confirm()`); the list of domains the agent does not enforce is only in the
`pending` answer.

Python has the same gap: `sdk/python/vrx/session.py:193-208` (`commit_confirmed` returns the confirm answer) and
`:234-235` (`transaction` with `confirm=None` accepts `partially-applied`).

**Failure scenario.** A `vrx_config` for a domain this agent build does not implement (P06 review M4) applies cleanly with
revision N. The user believes the change is enforced; it is only stored.

**Fix.**
- Parse `notApplied` (and `sync`) from the commit answer.
- Terraform: emit a warning diagnostic naming the domains, with an opt-in `fail_on_not_applied` provider flag.
- SDK: merge the pending result's `notApplied` into `tx.result`, or raise a typed `NotEnforced` warning or exception.

### M2 — sync state is ignored before and after the commit
The post-commit check is only `GET /state/system` succeeding (`client.go:372`, `session.py:292-293`). It never looks at
`sync.state`. Neither client checks the sync state before editing.

**Failure scenario.** Running is `unknown` or `degraded` (a lost Apply answer, or DEGRADED). Terraform keeps stacking confirmed
commits on a data plane in an unknown state and confirms each one because the API answers. With `confirm_timeout = 0`,
a `502 running-unknown` is reported as failure and the candidate is discarded. The API's reconcile may still promote that
document afterwards, so the next plan tries to Create again and gets "already exists — import it".

**Fix.**
- Refuse to start when `/state/system` reports sync ≠ `in-sync`, unless an explicit `allow_unsynced` is set.
- Do not confirm when the pending answer or the check shows sync ≠ `in-sync`.
- Document the `running-unknown` outcome.

### M3 — `quietDiscard` can wipe a concurrent same-user run's edits and makes Create report false success
`client.go:343-363`. The dirty check (`Diff`) and the edit are not atomic, and the candidate is per **user** (D-093).

**Failure scenario.** Pipelines A and B use keys of the same user.
1. A runs `Diff` (empty). B runs `Diff` (empty).
2. A PUTs /interfaces/x. B PUTs something invalid and gets 400.
3. B's `quietDiscard` drops A's staged edit.
4. A's second `Diff` is empty, so `Apply` returns `{Status:"unchanged"}` with no error.
5. `Create` writes state with `revision = 0` for an object that was never committed. The next refresh removes it.

The Python `transaction()` (`session.py:224-240`) has the same behaviour.

D-093 accepts the shared candidate as a limitation. The clients must still not report a successful Create or Update that
committed nothing.

**Fix.**
- In Create, and in Update where the plan changed `value`, treat `unchanged` as an error ("candidate was modified concurrently").
- Discard only when the candidate diff still consists of exactly our own pointer's changes.
- Put D-093's recommended practice (one service user per pipeline) into `docs/user/system/sdk-terraform-ansible.md`. Today the doc only says the clients refuse a dirty candidate, which suggests a safety that does not exist under a race.

### M4 — drift detection misses members added out of band, and the next PUT silently deletes them
`internal/provider/jsonvalue.go:65-96` and `config_resource.go:184-192`. State keeps the configured JSON while it is a *subset*
of live, so any extra object member counts as an "API default".

**Failure scenario.** `vrx_config` manages `/interfaces/loop10` with `{enabled, ipv4}`. An operator adds `description` and
`mtu: 9000` in the UI. `terraform plan` shows **No changes**. Later the user changes `ipv4`. The PUT replaces the whole node,
so `description` and `mtu` revert to their defaults, and the plan never showed that. The subset rule cannot tell a real
out-of-band member from a schema default.

**Fix.** After each apply, record the node the API actually stored (`GET` of running) in private state. On Read, report
drift when live ≠ that stored node, not when live ⊉ config. Alternatively, fetch the defaults the API fills in (a PUT of
the configured value into a scratch candidate path, or the schema defaults from the OpenAPI document) and compare against
config plus defaults.

### L1 — generator code injection (Python and Go) from crafted OpenAPI strings
- `sdk/python/tools/gen.py:22,170`: `info.title` and `info.version` are written raw into a `#` comment. A newline breaks out of the comment.
- `gen.py:228-230`: `path` is written raw into a `"""` docstring. Only `summary` has `"""` escaped, and a trailing backslash is not handled.
- `sdk/terraform/tools/genschema/main.go:72`: the same header problem in a `//` comment.
- `genschema/main.go:235-260,420-424`: `snake()`/`pascal()` keep arbitrary runes, and property keys become Go identifiers in `gen…Object` helper names.

Probe: `info.title = "VRX\nimport os; PWNED_HEADER = 1\n#"` plus a path `/x"""+str(__import__("os").getpid())+"""` produced
`operations.py` that parses with both payloads as live code. The OpenAPI document comes from our own build, and `--check`
plus review make this hard to exploit, but `gen.sh --openapi <file>` accepts any file.

**Fix.**
- Python: emit docstrings and comments through `repr()`, or strip control characters and `"""` and backslashes.
- Go: `strconv.Quote` in comments, or reject non-printable characters.
- Validate every identifier-producing name against `^[A-Za-z_][A-Za-z0-9_]*$` and fail generation otherwise.
- `ast.parse` / `format.Source` already run; also assert that the set of top-level names is the expected one.

### L2 — plain `http://` is accepted silently, so the API key crosses the network in clear
`client.go:92` and `session.py:69-70`. `insecure = true` warns but `http://` does not. The docs say "TLS certificates are verified
by default", which is true only when TLS is used at all.

**Fix.** Refuse `http://` for non-loopback hosts unless `allow_http = true` / `allow_insecure_http=True`, and warn when it is used.

### L3 — `sensitive_value` merges arrays by index, so a hash can attach to the wrong user
`jsonvalue.go:114-127`, and the docs example (`value = [admin, ops]`, `sensitive_value = [{}, {passwordHash}]`).

**Failure scenario.** Removing or reordering a user in `value` without editing `sensitive_value` moves the hash to a different
account. The plan cannot show it because the attribute is write-only.

**Fix.** Merge `management.users` by `username` (a keyed-array table next to `writeOnlyPointers`). At minimum, fail when the
array lengths differ or when a `sensitive_value` element has no matching element in `value`.

### L4 — candidate left dirty after a failed post-commit check or confirm
`client.go:372-378` and `session.py:236-237`. The API's `resolveReverted` clears the pending commit but not the candidate. After an
auto-revert, the candidate still holds our edits under our lock. Every later run fails with "dirty candidate" until a
human discards it.

**Fix.** Document the recovery in the error message (`POST /config/discard` after the deadline), or have the next `Apply`
recognise its own reverted edits (same pointer, same content) and discard them.

### L5 — docs and small correctness notes
- `docs/user/system/sdk-terraform-ansible.md` example `vrx_config "hostname"` on `/system/hostname`: Create refuses any node that already exists in running. If the appliance has a hostname (default or first commit), the documented example fails with "import it". Say "import first" for singleton pointers.
- `internal/provider/state_datasource.go:346`: the path regex allows `..` segments (`x/../../…`), so `vrx_state` can GET any API route the key can already read. This is not an escalation, but tighten it to known state names.
- `Revision` is `0` for a no-op commit, and `Read` sets `0` after import. That is harmless but undocumented in the attribute description.

## Summary
The Python SDK is solid:
- the key is never in repr or logs;
- redirects are refused;
- TLS verification is on by default and warns when disabled;
- errors are typed with pointers;
- the live evidence reproduces.

The Terraform provider works through the protocol harness and against the real API and VPP. However:
- H1: under a real Terraform CLI it would abort ("Provider produced invalid plan") in routine import and reformat paths, and the harness hides this;
- H2: it can still write a password hash into the state file;
- M1–M3: a commit that is not enforced, or not made at all, can be reported as success.

All of these are local fixes in `sdk/**` and need no API change.

H1 and H2 must be fixed before merge. M1–M4 should be fixed in this branch or tracked as follow-ups. L1–L5 can be follow-ups.

**APPROVE WITH CHANGES**
