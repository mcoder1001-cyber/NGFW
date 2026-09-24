# F-sdk-terraform-ansible — Python SDK + Terraform provider (Ansible left over)

Branch `task/F-sdk-terraform-ansible` (worktree `/root/ngfw-wt/F-sdk-terraform-ansible`, slot 5: `w5`, API :3500, DB `vrx_w5`,
Valkey db 5 prefix `vrx:w5:sdk:`). Base `main@78539ec`. Ran directly on the host. No terraform CLI on the host (not
installed, per the envelope) — the provider is driven through the Terraform plugin protocol by an in-repo harness
(see "Terraform without the CLI").

## What was built

| scope item | where | notes |
|---|---|---|
| 1 Python SDK | `sdk/python/` (package `vrx`) | **generated**: `tools/gen.py` (stdlib) → `vrx/_generated/operations.py` (41 operations = every operationId, typed params), `models.py` (341 TypedDicts: all components incl. the 13 config domains + every request/response body), `secrets.py` (write-only + secret-ref pointer patterns from the schema). **hand-written**: `VrxSession` (API key from arg/file/`VRX_API_KEY`, never in repr/logs; TLS verify on, `ca_file`, no redirects), `running/candidate/set/merge/delete/diff/validate/discard`, `commit(confirm=)`, `confirm`, `commit_confirmed(check=)`, `rollback`, `transaction()` (refuses a dirty candidate, commit+check+confirm on exit, discard on error), `state()`, `put_secret()`; problem+json → `ValidationError/BadRequest, Unauthorized, Forbidden, NotFound, Conflict(.lock), CommitFailed(.results), Unavailable(.sync)` with `.pointer/.errors`; `redact()` from the generated patterns. No runtime dependencies. |
| 2 Terraform provider | `sdk/terraform/` (own Go module `ngfw/sdk/terraform`, plugin framework v1.19, protocol 6) | provider `url/api_key(sensitive)/insecure=false/ca_file/confirm_timeout(60)/commit_comment`; **`vrx_config`** (pointer + JSON value; create refuses an existing node → import hint; import by pointer; semantic JSON + equivalent-pointer plan modifiers; defaults added by the API are not drift, a changed configured member is; `sensitive_value` = **write-only + sensitive** JSON overlay for `passwordHash`, `sensitive_value_version` to resend; write-only members inside `value` are refused at validate); **`vrx_interface`** with attributes **generated** from the `InterfacesConfig` JSON Schema (`tools/genschema` → `zz_interface_schema_gen.go`: defaults, nested `dhcp_client`, map-nested `subinterfaces`, snake_case↔camelCase table, secret-ref attributes marked Sensitive — none in interfaces today); **data `vrx_state`**. Every change = `client.Apply`: serialised per provider, dirty-candidate refusal, edit, commit `?confirm=N`, `/state/system` check, confirm; discard on any failure before the commit; unconfirmed on a failed check (agent reverts). |
| 3 Ansible | — | **not built** (D-085 "Ansible cut"; prompt priority 3) → questions #1 |
| 4 generation | `sdk/gen.sh [--check] [--openapi f]`, `sdk/test.sh` | build `@ngfw/api` → OpenAPI → both generators; `--check` fails on stale generated files. Not in `tools/ci.sh` (not owned) → manual gate, questions #2 |
| 5 docs | `docs/user/system/sdk-terraform-ansible.md` | transaction model, credentials/secrets, Python and Terraform worked examples (loopback + address, confirmed commit), what is not available |
| live env | `test/topology/sdk-terraform-ansible/live.sh up/down/run` | pg-test DB, real `vrx-agent` (owner w5, slot socket/state/metrics), API `dist/main.js` on :3500, admin login → API key (0600 in /run, never printed), `run` = up + cmd under `tools/lab lock shared` + down (stop by PID, Valkey keys by prefix, DB dropped) |

## How it was verified (real output)

### Live — `eval "$(tools/lab env 5)"; test/topology/sdk-terraform-ansible/live.sh run bash -c '<python live>; <terraform live>'`
Real API (P06) + real `vrx-agent` (P05) + VPP on this host. Environment up:
```
create role vrx_w5
create database vrx_w5 (owner vrx_w5)
check  vrx_w5 as vrx_w5 · PostgreSQL 18.6 (Ubuntu 18.6-0ubuntu0.26.04.1) on x86_64-pc-linux-gnu
ok     env /run/vrx-test/w5/pg.env (0600) · DSN postgres://vrx_w5:<redacted>@127.0.0.1:5432/vrx_w5
live: agent up (pid 2436509, owner w5, socket /run/vrx-test/w5/agent.sock)
live: api up (pid 2436544, http://127.0.0.1:3500)
live: API key 'sdk-live' role=admin id=2eabff7c-df1b-407d-8baa-83d1259fe2b1 → /run/vrx-test/w5/sdk-apikey (0600, not printed)
```
**Python SDK** (`sdk/python/.venv/bin/pytest -s sdk/python/tests/test_live.py`): prefixed loopback + address via
candidate → confirmed commit, read back from `/state/interfaces` and VPP, readdress, **rollback** to the first revision,
validation error with pointer, delete; the API key never appears in the SDK log:
```
== python live

[live] VrxSession(url='http://127.0.0.1:3500', verify=True) · API 0.1.0-dev · agent owner w5
[live] diff: [('add', '/interfaces/loop511', {'vrf': 'default', 'ipv4': ['10.5.111.1/24'], 'ipv6': [], 'enabled': True, 'promiscuous': False, 'subinterfaces': {}})]
[live] commit → status=confirmed revision=1 txn=fd72e9d9-c394-4da3-ba18-96405d2d5a7d
[live] /state/interfaces loop511: config={'ipv4': ['10.5.111.1/24'], 'vrf': 'default'} swIfIndex=4
[live] vppctl show int addr:
loop511 (dn):
  L3 10.5.111.1/24
[live] commit → status=confirmed revision=2
[live] rollback/1 → pending → confirm → confirmed revision=3
[live] /state/interfaces loop511 after rollback: config={'ipv4': ['10.5.111.1/24'], 'vrf': 'default'}
[live] vppctl show int addr:
loop511 (dn):
  L3 10.5.111.1/24
[live] invalid edit → BadRequest pointer=/interfaces/loop511/ipv4/0 (Invalid IPv4 range)
[live] delete → confirmed; in running: False; in state: False
[live] vppctl show int addr:
(no loop511 in `vppctl show int addr`)
[live] 37 SDK log records, API key present in them: False
.
1 passed in 1.79s
```
**Terraform provider** (`go -C sdk/terraform test -count=1 -v -run TestLive ./internal/provider`): plan → apply → refresh →
**second plan "No changes."** for `vrx_config` (create + update) and `vrx_interface` (create + update), `data.vrx_state`,
import by pointer, the password hash only via the write-only `sensitive_value` (not in plan text, not in state; the
API stored it — psql boolean `t`), destroy:
```
== terraform live
=== RUN   TestLive
$ terraform plan   (vrx_config)
  # vrx_config will be create
  + resource "vrx_config" {
      + id                       = (known after apply)
      + pointer                  = "/interfaces/loop521"
      + revision                 = (known after apply)
      + sensitive_value          = (write-only attribute)
      + value                    = "{\"enabled\":true,\"ipv4\":[\"10.5.121.1/24\"]}"
    }
[tf-live] apply → revision 5
$ terraform plan   (again, after refresh)
No changes. Your infrastructure matches the configuration.
$ terraform plan   (vrx_config)
  # vrx_config will be update in-place
  ~ resource "vrx_config" {
      ~ revision                 = 5 -> (known after apply)
      ~ value                    = "{\"enabled\":true,\"ipv4\":[\"10.5.121.1/24\"]}" -> "{\"enabled\":true,\"ipv4\":[\"10.5.121.2/24\"]}"
    }
[tf-live] apply → revision 6
$ terraform plan   (again, after refresh)
No changes. Your infrastructure matches the configuration.
[tf-live] data.vrx_state interfaces: loop521 config=map[ipv4:[10.5.121.2/24] vrf:default]
[tf-live] vppctl show int addr: loop521 (dn): L3 10.5.121.2/24
[tf-live] import /interfaces/loop521 → value={"enabled":true,"ipv4":["10.5.121.2/24"],"ipv6":[],"promiscuous":false,"subinterfaces":{},"vrf":"default"}
$ terraform plan   (vrx_interface)
  # vrx_interface will be create
  + resource "vrx_interface" {
      + description              = "terraform w5"
      + enabled                  = true
      + id                       = (known after apply)
      + ipv4                     = ["10.5.122.1/24"]
      + ipv6                     = []
      + mtu                      = 1500
      + name                     = "loop522"
      + promiscuous              = false
      + revision                 = (known after apply)
      + subinterfaces            = {}
      + vrf                      = "default"
    }
[tf-live] apply → revision 7
$ terraform plan   (again, after refresh)
No changes. Your infrastructure matches the configuration.
$ terraform plan   (vrx_interface)
  # vrx_interface will be update in-place
  ~ resource "vrx_interface" {
      ~ mtu                      = 1500 -> 9000
      ~ revision                 = 7 -> (known after apply)
    }
[tf-live] apply → revision 8
$ terraform plan   (again, after refresh)
No changes. Your infrastructure matches the configuration.
[tf-live] vppctl show int addr: loop522 (dn): L3 10.5.122.1/24
$ terraform plan   (vrx_config)
  # vrx_config will be update in-place
  ~ resource "vrx_config" {
      ~ revision                 = 0 -> (known after apply)
      ~ sensitive_value_version  = null -> 1
      ~ value                    = "[]" -> "[{\"username\":\"admin\",\"role\":\"admin\"},{\"username\":\"w5tf\",\"role\":\"operator\"}]"
    }
[tf-live] apply → revision 9
$ terraform plan   (again, after refresh)
No changes. Your infrastructure matches the configuration.
[tf-live] hash inside value → validation: write-only member in value: /management/users/0/passwordHash is write-only (a secret): move it into sensitive_value so it never appears in plans or state
[tf-live] state after apply: sensitive_value=<nil> (write-only), hash in state: false
[tf-live] app_user w5tf has a password hash: t
$ terraform plan   (vrx_config)
  # vrx_config will be update in-place
  ~ resource "vrx_config" {
      ~ revision                 = 9 -> (known after apply)
      ~ value                    = "[{\"username\":\"admin\",\"role\":\"admin\"},{\"username\":\"w5tf\",\"role\":\"operator\"}]" -> "[{\"username\":\"admin\",\"role\":\"admin\"}]"
    }
[tf-live] apply → revision 10
$ terraform plan   (again, after refresh)
No changes. Your infrastructure matches the configuration.
[tf-live] app_user w5tf still present: f
$ terraform plan   (vrx_interface)
  # vrx_interface will be destroy
  - resource "vrx_interface" {
      - description              = "terraform w5"
      - enabled                  = true
      - id                       = "loop522"
      - ipv4                     = ["10.5.122.1/24"]
      - ipv6                     = []
      - mtu                      = 9000
      - name                     = "loop522"
      - promiscuous              = false
      - revision                 = 8
      - subinterfaces            = {}
      - vrf                      = "default"
    }
[tf-live] destroy applied (confirmed commit)
$ terraform plan   (vrx_config)
  # vrx_config will be destroy
  - resource "vrx_config" {
      - id                       = "/interfaces/loop521"
      - pointer                  = "/interfaces/loop521"
      - revision                 = 6
      - value                    = "{\"enabled\":true,\"ipv4\":[\"10.5.121.2/24\"]}"
    }
[tf-live] destroy applied (confirmed commit)
[tf-live] after destroy: (no loop521 in vppctl show int addr) (no loop522 in vppctl show int addr)
--- PASS: TestLive (2.63s)
PASS
ok  	ngfw/sdk/terraform/internal/provider	2.672s
```
Teardown:
```
live: api stopped (pid 2436544)
live: agent stopped (pid 2436509)
live: deleted 4 Valkey keys vrx:w5:sdk:* in db 5
drop   database vrx_w5
drop   role vrx_w5
ok     nothing named vrx_w5 / vrx_w5 remains
```
Afterwards: `vppctl show int | grep -cE "loop5(11|21|22)\b"` → `0`; `/run/vrx-test/w5/` holds only the pre-existing
`df8-globals.lock`; `valkey-cli -n 5 --scan --pattern 'vrx:w5:*' | wc -l` → `0`; database `vrx_w5` gone;
`grep -c argon2` over the live log and the API log → `0`.

### Unit — `sdk/test.sh`
Python (mocked HTTP layer, `tests/fake.py`) — `pytest -rA`:
```
  ok  tests/test_generated.py::test_every_operation_has_a_method
  ok  tests/test_generated.py::test_generated_method_builds_the_request
  ok  tests/test_generated.py::test_models_cover_the_config_domains
  ok  tests/test_generated.py::test_secret_patterns_generated_from_the_schema
  ok  tests/test_session.py::test_api_key_header_and_no_key_in_repr
  ok  tests/test_session.py::test_key_from_file_and_env
  ok  tests/test_session.py::test_rejects_credentials_in_url
  ok  tests/test_session.py::test_tls_verification_is_on_by_default
  ok  tests/test_session.py::test_pointer_to_url_encoding
  ok  tests/test_session.py::test_body_less_posts_send_no_content_type
  ok  tests/test_session.py::test_validation_problem_becomes_typed_error_with_pointer
  ok  tests/test_session.py::test_status_to_exception[401-Unauthorized]
  ok  tests/test_session.py::test_status_to_exception[403-Forbidden]
  ok  tests/test_session.py::test_status_to_exception[404-NotFound]
  ok  tests/test_session.py::test_status_to_exception[409-Conflict]
  ok  tests/test_session.py::test_status_to_exception[422-CommitFailed]
  ok  tests/test_session.py::test_status_to_exception[503-Unavailable]
  ok  tests/test_session.py::test_status_to_exception[500-ApiError]
  ok  tests/test_session.py::test_non_json_error_body
  ok  tests/test_session.py::test_transaction_commits_with_confirm_then_confirms
  ok  tests/test_session.py::test_transaction_refuses_dirty_candidate
  ok  tests/test_session.py::test_transaction_discards_on_exception_and_on_failed_commit
  ok  tests/test_session.py::test_failed_check_does_not_confirm
  ok  tests/test_session.py::test_unchanged_transaction_discards_without_commit
  ok  tests/test_session.py::test_rollback_and_revisions
  ok  tests/test_session.py::test_logs_never_carry_key_or_write_only_values
  ok  tests/test_session.py::test_redact_helpers
  skip [1] tests/test_live.py:63: live run: VRX_INTEGRATION=1 + VRX_SDK_URL + VRX_SDK_API_KEY_FILE (test/topology/sdk-terraform-ansible/live.sh run …)
======================== 27 passed, 1 skipped in 0.20s =========================
```
Terraform provider (httptest fake API with defaults/validation/redaction, driven through the plugin protocol) — `go test -v`, plus `gofmt -l` empty, `go vet`, `golangci-lint run` → `0 issues.`:
```
--- PASS: TestPointers (0.00s)
--- PASS: TestNoRedirectNoKeyLeak (0.01s)
--- PASS: TestProblemParsing (0.00s)
ok  	ngfw/sdk/terraform/internal/client	0.029s
--- PASS: TestJSONSubset (0.00s)
--- PASS: TestDeepMergeAndWriteOnlyLeaves (0.00s)
--- SKIP: TestLive (0.00s)
--- PASS: TestConfigResourceLifecycle (0.07s)
--- PASS: TestConfigImportAndExisting (0.01s)
--- PASS: TestConfigSecretsNeverInPlanOrState (0.02s)
--- PASS: TestDirtyCandidateIsRefused (0.01s)
--- PASS: TestValidationErrorKeepsPointerAndDiscards (0.01s)
--- PASS: TestFailedPostCommitCheckDoesNotConfirm (0.01s)
--- PASS: TestNoConfirmWhenTimeoutZero (0.01s)
--- PASS: TestProviderConfiguration (0.01s)
--- PASS: TestInterfaceResource (0.10s)
--- PASS: TestStateDataSource (0.01s)
ok  	ngfw/sdk/terraform/internal/provider	0.309s
```

### Generated output is clean — `sdk/gen.sh --check` (full API build → OpenAPI → both generators)
```
$ tsc -p tsconfig.build.json
api: OpenAPI written to /root/ngfw-wt/F-sdk-terraform-ansible/sdk/openapi.json
python sdk: 41 operations, 341 models, 1 write-only + 22 secret-ref pointer patterns → /root/ngfw-wt/F-sdk-terraform-ansible/sdk/python/vrx/_generated
terraform: vrx_interface 6 JSON names, 1 helpers; 1 write-only + 22 secret-ref pointer patterns → internal/provider
gen: clean — sdk/python/vrx/_generated sdk/terraform/internal/provider/zz_*_gen.go
```

### CI gate — `tools/ci.sh --base main` (HEAD 69109ac; later commits touch docs/status only)
```
== contract guard: HEAD vs main ==
no contract files changed in the 4 commit(s) of HEAD since main (78539ec)

== tools (golangci-lint, gitleaks) ==
golangci-lint 2.13.2
gitleaks 8.30.1

== install (pnpm --frozen-lockfile --prefer-offline) ==
Lockfile is up to date, resolution step is skipped Done in 176ms using pnpm v12.5.1 

== generate + generated-output gate ==
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated

== forbidden patterns (+ gitleaks) ==
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: gitleaks — scanned ~346226 bytes (346.23 KB) in 1.19s no leaks found 

== lint · typecheck · unit tests · build (turbo) ==
Tasks:    30 successful, 30 total Cached:    24 cached, 30 total Time:    1m22.13s  

== apps/agent: make lint test build ==
ok  	ngfw/agent/cmd/vrx-startupgen	1.590s; ok  	ngfw/agent/internal/agent	9.459s; ok  	ngfw/agent/internal/contracttest	2.294s; ok  	ngfw/agent/internal/descriptors/abf	1.161s; ok  	ngfw/agent/internal/descriptors/acl	1.243s; ok  	ngfw/agent/internal/descriptors/adl	1.110s; ok  	ngfw/agent/internal/descriptors/af_packet	1.125s; ok  	ngfw/agent/internal/descriptors/arp	1.085s; ok  	ngfw/agent/internal/descriptors/bfd	1.212s; ok  	ngfw/agent/internal/descriptors/bond	1.128s; ok  	ngfw/agent/internal/descriptors/classify	1.167s; ok  	ngfw/agent/internal/descriptors/cnat	1.143s; 

== test/ Go modules, unit mode (test/integration/smoke) ==
test/integration/smoke: gofmt ok · go vet ok · ok  	ngfw/test/integration/smoke	0.019s; 
integration tests inside these modules skip here (VRX_INTEGRATION unset); 'tools/ci.sh full' runs them on the CI slot

== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m00s
  generate + generated-output gate                   1m20s
  forbidden patterns (+ gitleaks)                    0m04s
  lint · typecheck · unit tests · build (turbo)   1m23s
  apps/agent: make lint test build                   1m00s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  mode quick · wall time 3m52s · logs /root/ngfw-wt/logs/ci/F-sdk-terraform-ansible-20260924-033506-2440431

CI GATE PASSED
```

## Terraform without the CLI
`terraform` is not on the host and was not installed (envelope). `terraform-plugin-testing` needs the CLI, so
`sdk/terraform/internal/tfharness` drives the provider through `tfprotov6` like Terraform core: `ValidateResourceConfig`
(with the write-only client capability), `PlanResourceChange` with core's proposed-new-state rule (config wins,
computed-null takes prior, nested single/map objects merge, write-only never proposed), `ApplyResourceChange` followed
by core's **post-apply consistency check** ("provider produced inconsistent result"), `ReadResource` (refresh),
`ImportResourceState`, `ReadDataSource`. The `$ terraform plan` blocks above are the harness's rendering of each
PlanResourceChange (Terraform's masking: `(sensitive value)`, `(write-only attribute)`, `(known after apply)`) — an
emulation, not CLI output. A real CLI run is questions #7.

## Dependencies (new)
Python — runtime: none (stdlib). Test tooling, `sdk/python/requirements-dev.lock` (hash-pinned, `pip install --require-hashes`; `sdk/python/lock.sh lock` regenerates it):
pytest 9.1.1 (MIT) · pluggy 1.6.0 (MIT) · iniconfig 2.3.0 (MIT) · packaging 26.3 (Apache-2.0 OR BSD-2-Clause) · Pygments 2.21.0 (BSD-2-Clause).

Go — `sdk/terraform/go.mod` + `go.sum` (Go modules via proxy.golang.org, `GOTOOLCHAIN=local`); linked modules:
terraform-plugin-framework v1.19.0, terraform-plugin-go v0.31.0, terraform-plugin-log v0.10.0, go-plugin v1.7.0,
go-uuid v1.0.3, terraform-registry-address v0.4.0, terraform-svchost v0.1.1, yamux v0.1.2 (all HashiCorp, **MPL-2.0**);
go-hclog v1.6.3 (MIT); fatih/color v1.18.0, mattn/go-colorable v0.1.14, mattn/go-isatty v0.0.20,
mitchellh/go-testing-interface v1.14.1 (MIT); oklog/run v1.1.0, google.golang.org/grpc v1.79.2, genproto/googleapis/rpc (Apache-2.0);
golang/protobuf v1.5.4, google.golang.org/protobuf v1.36.11, golang.org/x/net v0.48.0, x/sys v0.39.0, x/text v0.32.0 (BSD-3-Clause);
vmihailenco/msgpack/v5 v5.4.1, vmihailenco/tagparser/v2 v2.0.0 (BSD-2-Clause).
MPL-2.0 is file-level copyleft: fine for a provider binary that is not modified-and-distributed as source; noted for the licence register.

## Out of scope / left undone
- **Ansible collection** (`sdk/ansible/`, `vrx.appliance`: `vrx_config`, `vrx_commit`, `vrx_facts`) — not started (D-085 cut; questions #1).
- A real `terraform plan/apply/destroy` with the CLI (questions #7); registry publishing (out of scope).
- `sdk/test.sh` / `sdk/gen.sh --check` not wired into `tools/ci.sh` (questions #2).
- Typed resources for domains other than interfaces (out of scope; `vrx_config` covers them). No resource for the secret store (`POST /secrets`) in Terraform; the SDK has `put_secret()`.
- Per-run batching of Terraform changes into one candidate (questions #3).

## Decisions (for the LOG)
| id | decision | options | why |
|---|---|---|---|
| D-SDK-1 | Python models/operations come from an **own stdlib generator** (`sdk/python/tools/gen.py`, TypedDicts + one method per operationId), not openapi-python-client / datamodel-code-generator | (a) openapi-python-client (attrs + httpx) (b) datamodel-code-generator (pydantic) (c) own generator | zero runtime deps for the appliance/Ansible side, deterministic output for the `--check` gate, OpenAPI 3.1 shapes of this document (propertyNames maps, const, anyOf-null) handled explicitly; ~250 lines |
| D-SDK-2 | Every Terraform resource change = one **confirmed commit** (default 60 s), serialised by a provider mutex; post-commit check = `GET /state/system`; failed check → not confirmed (agent reverts) | (a) commit per resource (b) one candidate per run | (b) needs an API session concept (questions #3); prompt default |
| D-SDK-3 | Both clients **refuse a dirty candidate** instead of committing someone else's staged edits | (a) refuse (b) commit everything (c) discard | (b) commits foreign changes, (c) destroys them |
| D-SDK-4 | `vrx_config` drift rule: state keeps the configured JSON while it is a **subset** of the running node (API-filled defaults, element-wise in arrays); otherwise the whole node is shown | (a) exact match (b) subset (c) ignore reads | (a) perpetual diffs from defaults, (c) no drift detection |
| D-SDK-5 | Write-only config members (`passwordHash`) only via a Terraform **write-only** attribute (`sensitive_value`, merged over `value`, TF ≥ 1.11); inside `value` they are refused | (a) Sensitive `value` (b) separate write-only attribute | (a) would hide every diff of the resource and still store the hash in state |
| D-SDK-6 | Terraform verified through an in-repo plugin-protocol harness, since the CLI is absent and must not be installed | (a) install CLI (b) unit only (c) protocol harness | envelope forbids (a); (c) exercises plan/apply/refresh/import against the real API |
| D-SDK-7 | Provider module path `ngfw/sdk/terraform`, provider address `registry.terraform.io/vrx/vrx` (dev_overrides only) | — | local-only, mirrors `ngfw/agent` |

## Open questions
`docs/status/tasks/F-sdk-terraform-ansible-questions.md` (7 items: Ansible, CI hook, batching, per-key lock, OpenAPI
pointer parameter, users via automation, real CLI run).


## Review fixes (docs/status/tasks/F-sdk-terraform-ansible-review.md, a2b2ad1)

Round started with `git merge main` (9764d69). The partial edits from before the usage-limit interruption were
salvaged by the manager in 18c718f and completed here.

| finding | fix | where | test |
|---|---|---|---|
| **H1** plan modifiers rewrote configured values | `samePointer` and `semanticJSON` are deleted. `value` is now a custom string type with **semantic equality** (`jsontype.go`), which the framework applies on Read/Apply. Plans are never rewritten, so a reformatted value is a harmless in-place update that commits nothing. `pointer` must be canonical (`ValidateConfig`), and imports store the canonical form. **The harness enforces Terraform core's plan-validity rule** (`planValid`: configured non-computed or optional+computed values are planned exactly as configured, nested attributes recurse with their own flags, write-only attributes are planned null). It also threads private state the way core does. | `config_resource.go`, `jsontype.go`, `tfharness/harness.go` | `TestPlanValidRejectsRewrittenConfiguredValues`; `TestConfigImportAndExisting` (the review's import-then-reformat probe); `TestConfigResourceLifecycle` (reformat gives a valid plan, revision 0, and the next plan is empty); `TestNonCanonicalPointerIsRefused`. Every unit test and the live run now go through `planValid`. |
| **H2** write-only guard skipped when `value` is unknown at validate | `checkValue` also runs in `ModifyPlan` (the planned value is known there) and in `body()` before any HTTP call | `config_resource.go` | `TestWriteOnlyGuardWhenValueUnknownAtValidate` (the review's probe): unknown at validate → refused at plan; unknown at plan but known at apply → refused before the PUT, with no hash in state and no PUT or commit |
| **M1** not-applied reported as success | `CommitResult` has `NotApplied` and `Sync`, and the confirm answer inherits `notApplied` from the pending answer. Terraform: a **warning** by default, an error with `fail_on_not_applied = true`. SDK: `NotEnforcedWarning`, or `NotEnforced` with `require_enforced=True`; `tx.result["notApplied"]` | `client.go`, `provider.go`, `session.py`, `errors.py` | `TestNotAppliedWarning`, `TestNotAppliedIsAnErrorWhenAsked`, `test_not_enforced_warns_or_raises` |
| **M2** sync state ignored | Before editing, `/state/system` must report `in-sync` and no pending commit. Before confirming, the pending answer's `sync` and `/state/system` must both report `in-sync`; otherwise the commit is not confirmed. `allow_unsynced` overrides this. A 502/504 at commit time returns "outcome UNKNOWN" and the candidate is not discarded. | `client.go`, `session.py` | `TestUnsyncedApplianceIsRefused`, `test_transaction_refuses_unsynced_appliance`, `test_pending_answer_out_of_sync_is_not_confirmed` |
| **M3** racy discard / false success | The candidate is discarded **only** when every change in it lies inside our own pointer(s). If a change outside our pointer appears after the edit, the client returns `ErrConcurrent` / `ConcurrentEdit` and neither commits nor discards. A Create that commits nothing is an error; an Update that commits nothing is an error unless the node already holds the value. The user doc now says **one service user per pipeline** (D-093). | `client.go`, both resources, `session.py`, user doc | `TestConcurrentEditIsNotCommittedNorDiscarded`, `TestCreateThatCommittedNothingFails`, `TestDirtyCandidateIsRefusedAndNeverDiscarded`, `test_concurrent_edit_is_neither_committed_nor_discarded`, `test_transaction_discards_only_its_own_changes` |
| **M4** drift misses added members | After each write, `vrx_config` records the node as the API stored it, in **private state**. Read reports drift when the live node differs from that record, so added members count too, and the plan shows the PUT removing them. `vrx_interface` refuses an update when the live object has members the generated schema does not know, and warns about them on Read. | `config_resource.go`, `interface_resource.go` | `TestConfigResourceLifecycle` (an out-of-band `mtu` shows as a planned removal), `TestInterfaceResource` (`futureField` → update refused) |
| L1 generator injection | Python: headers and comments go through `clean()`, docstrings are `repr()` literals, and raw operationIds, parameter names and schema names must be identifiers. Every generated module is re-parsed and must contain exactly the expected top-level names. Go: `generate()` returns errors instead of exiting, comments are sanitised, identifiers must match `^[A-Za-z_][A-Za-z0-9_]*$`, and the output passes `format.Source`. | `tools/gen.py`, `tools/genschema/main.go` | `test_gen_hostile.py` (the payload stays inert on import; hostile identifiers fail generation), `TestHostileSpecCannotInjectCode`, `TestHostilePropertyNameFailsGeneration` |
| L2 plain http | `http://` to a non-loopback host is refused unless `allow_http` is set, in which case the client warns | `client.go`, `provider.go`, `session.py` | `TestProviderConfiguration`, `test_http_refused_for_remote_hosts` |
| L3 index merge of hashes | Keyed arrays are generated from `x-vrx-ui.itemKey` (`/management/users` → `username`). `sensitive_value` elements are matched by that key, and a missing or unmatched key is an error. Unkeyed arrays must have the same length as in `value`. | `jsonvalue.go` `mergeSensitive`, `zz_secrets_gen.go` | `TestMergeSensitiveByKeyNeverByIndex`, `TestConfigSecretsNeverInPlanOrState` |
| L4 dirty candidate after auto-revert | The not-confirmed error names the deadline and how to recover. The next Terraform apply recognises its own leftover edit (changes only under our pointer, and the candidate node contains what we want) and re-uses it. | `client.go` `ownLeftover` | `TestOwnLeftoverAfterRevertIsReused`, `TestFailedPostCommitCheckDoesNotConfirm` |
| L5 docs / small | Singleton pointers (`/system/hostname`, `/management/users`) now say "import first". `vrx_state.path` is restricted to the six state names. The meaning of `revision` is documented. The user doc gained sections on the per-user candidate, sync state, stored-but-not-enforced commits, unconfirmed commits and http. | user doc, `state_datasource.go` | `TestStateDataSource` (`../config` and `auth` are refused) |

### `sdk/test.sh`
```
== python sdk
......s............................                                      [100%]
34 passed, 1 skipped in 1.05s
== terraform provider
0 issues.
?   	ngfw/sdk/terraform	[no test files]
ok  	ngfw/sdk/terraform/internal/client	0.036s
ok  	ngfw/sdk/terraform/internal/provider	0.640s
ok  	ngfw/sdk/terraform/internal/tfharness	0.026s
ok  	ngfw/sdk/terraform/tools/genschema	0.019s
sdk: OK
```
Provider, harness and generator tests (`go test -v ./...`):
```
--- PASS: TestPointers (0.00s)
--- PASS: TestNoRedirectNoKeyLeak (0.01s)
--- PASS: TestProblemParsing (0.00s)
ok  	ngfw/sdk/terraform/internal/client	0.031s
--- PASS: TestJSONSubset (0.00s)
--- PASS: TestMergeSensitiveByKeyNeverByIndex (0.00s)
--- PASS: TestWriteOnlyLeaves (0.00s)
--- SKIP: TestLive (0.00s)
--- PASS: TestConfigResourceLifecycle (0.09s)
--- PASS: TestNonCanonicalPointerIsRefused (0.00s)
--- PASS: TestConfigImportAndExisting (0.03s)
--- PASS: TestConfigSecretsNeverInPlanOrState (0.03s)
--- PASS: TestWriteOnlyGuardWhenValueUnknownAtValidate (0.00s)
--- PASS: TestDirtyCandidateIsRefusedAndNeverDiscarded (0.01s)
--- PASS: TestConcurrentEditIsNotCommittedNorDiscarded (0.01s)
--- PASS: TestCreateThatCommittedNothingFails (0.01s)
--- PASS: TestOwnLeftoverAfterRevertIsReused (0.02s)
--- PASS: TestValidationErrorKeepsPointer (0.01s)
--- PASS: TestFailedPostCommitCheckDoesNotConfirm (0.01s)
--- PASS: TestUnsyncedApplianceIsRefused (0.02s)
--- PASS: TestNotAppliedIsAnErrorWhenAsked (0.01s)
--- PASS: TestNotAppliedWarning (0.02s)
--- PASS: TestNoConfirmWhenTimeoutZero (0.01s)
--- PASS: TestProviderConfiguration (0.01s)
--- PASS: TestInterfaceResource (0.09s)
--- PASS: TestStateDataSource (0.01s)
ok  	ngfw/sdk/terraform/internal/provider	0.463s
--- PASS: TestPlanValidRejectsRewrittenConfiguredValues (0.00s)
ok  	ngfw/sdk/terraform/internal/tfharness	0.029s
--- PASS: TestHostileSpecCannotInjectCode (0.00s)
--- PASS: TestHostilePropertyNameFailsGeneration (0.00s)
ok  	ngfw/sdk/terraform/tools/genschema	0.017s
```

### `sdk/gen.sh --check` (full API build of the merged tree)
```
api: OpenAPI written to /root/ngfw-wt/F-sdk-terraform-ansible/sdk/openapi.json
python sdk: 41 operations, 341 models, 1 write-only + 22 secret-ref pointer patterns → /root/ngfw-wt/F-sdk-terraform-ansible/sdk/python/vrx/_generated
terraform: vrx_interface 6 JSON names, 1 helpers; 1 write-only + 22 secret-ref pointer patterns, 4 keyed arrays → internal/provider
gen: clean — sdk/python/vrx/_generated sdk/terraform/internal/provider/zz_*_gen.go
```

### Live on slot 5 — real API, vrx-agent and VPP (`live.sh run …`: the Python live test, then `TestLive` through the plan-validity-enforcing harness)
```
create role vrx_w5
create database vrx_w5 (owner vrx_w5)
check  vrx_w5 as vrx_w5 · PostgreSQL 18.6 (Ubuntu 18.6-0ubuntu0.26.04.1) on x86_64-pc-linux-gnu
ok     env /run/vrx-test/w5/pg.env (0600) · DSN postgres://vrx_w5:<redacted>@127.0.0.1:5432/vrx_w5
live: agent up (pid 2637373, owner w5, socket /run/vrx-test/w5/agent.sock)
live: api up (pid 2637675, http://127.0.0.1:3500)
live: API key 'sdk-live' role=admin id=963c36e4-b16e-4b63-bfce-2769a53fc6d9 → /run/vrx-test/w5/sdk-apikey (0600, not printed)
== python live

[live] VrxSession(url='http://127.0.0.1:3500', verify=True) · API 0.1.0-dev · agent owner w5
[live] diff: [('add', '/interfaces/loop511', {'vrf': 'default', 'ipv4': ['10.5.111.1/24'], 'ipv6': [], 'enabled': True, 'promiscuous': False, 'subinterfaces': {}})]
[live] commit → status=confirmed revision=1 txn=f9d5e6d2-47ee-40c9-a8d6-779cdd2084e5
[live] /state/interfaces loop511: config={'ipv4': ['10.5.111.1/24'], 'vrf': 'default'} swIfIndex=4
[live] vppctl show int addr:
loop511 (dn):
  L3 10.5.111.1/24
[live] commit → status=confirmed revision=2
[live] rollback/1 → pending → confirm → confirmed revision=3
[live] /state/interfaces loop511 after rollback: config={'ipv4': ['10.5.111.1/24'], 'vrf': 'default'}
[live] vppctl show int addr:
loop511 (dn):
  L3 10.5.111.1/24
[live] invalid edit → BadRequest pointer=/interfaces/loop511/ipv4/0 (Invalid IPv4 range)
[live] delete → confirmed; in running: False; in state: False
[live] vppctl show int addr:
(no loop511 in `vppctl show int addr`)
[live] 41 SDK log records, API key present in them: False
.
1 passed in 2.66s
== terraform live
=== RUN   TestLive
$ terraform plan   (vrx_config)
  # vrx_config will be create
  + resource "vrx_config" {
      + id                       = (known after apply)
      + pointer                  = "/interfaces/loop521"
      + revision                 = (known after apply)
      + sensitive_value          = (write-only attribute)
      + value                    = "{\"enabled\":true,\"ipv4\":[\"10.5.121.1/24\"]}"
    }
[tf-live] apply → revision 5
$ terraform plan   (again, after refresh)
No changes. Your infrastructure matches the configuration.
$ terraform plan   (vrx_config)
  # vrx_config will be update in-place
  ~ resource "vrx_config" {
      ~ revision                 = 5 -> (known after apply)
      ~ value                    = "{\"enabled\":true,\"ipv4\":[\"10.5.121.1/24\"]}" -> "{\"enabled\":true,\"ipv4\":[\"10.5.121.2/24\"]}"
    }
[tf-live] apply → revision 6
$ terraform plan   (again, after refresh)
No changes. Your infrastructure matches the configuration.
[tf-live] data.vrx_state interfaces: loop521 config=map[ipv4:[10.5.121.2/24] vrf:default]
[tf-live] vppctl show int addr: loop521 (dn): L3 10.5.121.2/24
[tf-live] import /interfaces/loop521 → value={"enabled":true,"ipv4":["10.5.121.2/24"],"ipv6":[],"promiscuous":false,"subinterfaces":{},"vrf":"default"}
$ terraform plan   (vrx_interface)
  # vrx_interface will be create
  + resource "vrx_interface" {
      + description              = "terraform w5"
      + enabled                  = true
      + id                       = (known after apply)
      + ipv4                     = ["10.5.122.1/24"]
      + ipv6                     = []
      + mtu                      = 1500
      + name                     = "loop522"
      + promiscuous              = false
      + revision                 = (known after apply)
      + subinterfaces            = {}
      + vrf                      = "default"
    }
[tf-live] apply → revision 7
$ terraform plan   (again, after refresh)
No changes. Your infrastructure matches the configuration.
$ terraform plan   (vrx_interface)
  # vrx_interface will be update in-place
  ~ resource "vrx_interface" {
      ~ mtu                      = 1500 -> 9000
      ~ revision                 = 7 -> (known after apply)
    }
[tf-live] apply → revision 8
$ terraform plan   (again, after refresh)
No changes. Your infrastructure matches the configuration.
[tf-live] vppctl show int addr: loop522 (dn): L3 10.5.122.1/24
$ terraform plan   (vrx_config)
  # vrx_config will be update in-place
  ~ resource "vrx_config" {
      ~ revision                 = 0 -> (known after apply)
      ~ sensitive_value_version  = null -> 1
      ~ value                    = "[]" -> "[{\"username\":\"admin\",\"role\":\"admin\"},{\"username\":\"w5tf\",\"role\":\"operator\"}]"
    }
[tf-live] apply → revision 9
$ terraform plan   (again, after refresh)
No changes. Your infrastructure matches the configuration.
[tf-live] hash inside value → validation: write-only member in value: /management/users/0/passwordHash is write-only (a secret): move it into sensitive_value so it never appears in plans or state
[tf-live] state after apply: sensitive_value=<nil> (write-only), hash in state: false
[tf-live] app_user w5tf has a password hash: t
$ terraform plan   (vrx_config)
  # vrx_config will be update in-place
  ~ resource "vrx_config" {
      ~ revision                 = 9 -> (known after apply)
      ~ value                    = "[{\"username\":\"admin\",\"role\":\"admin\"},{\"username\":\"w5tf\",\"role\":\"operator\"}]" -> "[{\"username\":\"admin\",\"role\":\"admin\"}]"
    }
[tf-live] apply → revision 10
$ terraform plan   (again, after refresh)
No changes. Your infrastructure matches the configuration.
[tf-live] app_user w5tf still present: f
$ terraform plan   (vrx_interface)
  # vrx_interface will be destroy
  - resource "vrx_interface" {
      - description              = "terraform w5"
      - enabled                  = true
      - id                       = "loop522"
      - ipv4                     = ["10.5.122.1/24"]
      - ipv6                     = []
      - mtu                      = 9000
      - name                     = "loop522"
      - promiscuous              = false
      - revision                 = 8
      - subinterfaces            = {}
      - vrf                      = "default"
    }
[tf-live] destroy applied (confirmed commit)
$ terraform plan   (vrx_config)
  # vrx_config will be destroy
  - resource "vrx_config" {
      - id                       = "/interfaces/loop521"
      - pointer                  = "/interfaces/loop521"
      - revision                 = 6
      - value                    = "{\"enabled\":true,\"ipv4\":[\"10.5.121.2/24\"]}"
    }
[tf-live] destroy applied (confirmed commit)
[tf-live] after destroy: (no loop521 in vppctl show int addr) (no loop522 in vppctl show int addr)
--- PASS: TestLive (2.99s)
PASS
ok  	ngfw/sdk/terraform/internal/provider	3.045s
live: api stopped (pid 2637675)
live: agent stopped (pid 2637373)
live: deleted 4 Valkey keys vrx:w5:sdk:* in db 5
drop   database vrx_w5
drop   role vrx_w5
ok     nothing named vrx_w5 / vrx_w5 remains
```
Afterwards: `vppctl show int | grep -cE "loop5(11|21|22)\b"` → `0`, 0 `vrx:w5:*` keys, `/run/vrx-test/w5` holds only
`df8-globals.lock`, `vrx_w5` dropped, both processes stopped by PID.

### CI gate — `tools/ci.sh --base main`
```
== VRX CI gate: quick ==
worktree  /root/ngfw-wt/F-sdk-terraform-ansible
branch    task/F-sdk-terraform-ansible @ 11493f3   (base: main)
tools     node v22.23.2 · pnpm 12.5.1 · go1.26.0 · buf 1.73.0 · golangci-lint 2.13.2 (pinned) · gitleaks 8.30.1 (pinned)
caches    pnpm store /root/.local/share/pnpm/store/v11 · turbo /root/.cache/vrx-turbo · go /root/.cache/go-build
logs      /root/ngfw-wt/logs/ci/F-sdk-terraform-ansible-20260924-043707-2707979
...
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   1m33s
  forbidden patterns (+ gitleaks)                    0m03s
  lint · typecheck · unit tests · build (turbo)   1m26s
  apps/agent: make lint test build                   0m27s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  warnings:
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      merge main into task/F-sdk-terraform-ansible
      review(F-sdk-terraform-ansible): findings
  mode quick · wall time 3m35s · logs /root/ngfw-wt/logs/ci/F-sdk-terraform-ansible-20260924-043707-2707979

CI GATE PASSED
```
The two warnings refer to the merge commit and the manager's `review(...)` commit.

### Still open
- A real terraform CLI run (terraform ≥ 1.11) is still needed before release (questions #7). The harness now checks
  plan validity, post-apply consistency and private state, but it remains an emulation.
- The per-user candidate is still a documented limitation (D-093). The ownership checks narrow the race but cannot
  close it without per-key candidates in the API (P06 tech-debt).
