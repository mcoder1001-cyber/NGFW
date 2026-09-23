# Task: F-sdk-terraform-ansible — Python SDK, Terraform provider, Ansible collection   (prepend 00-CONTEXT.md)

## Goal
Automation clients for the VRX REST API (WBS D8.9 in `plan/wbs.csv`, T2): a Python SDK, a Terraform provider and an Ansible collection,
all **generated from or driven by the OpenAPI document** that P06 produces, so new schema domains need no hand-written client code.
The three together do not fit a 10 h box: priority is **(1) Python SDK complete, (2) Terraform provider with the generic
config-pointer resource + one typed resource family (interfaces), (3) Ansible collection with a generic `vrx_config` module** — stop at the
time box and list what is left.

## Inputs to read first
- `apps/api/src/openapi.ts` (`node dist/openapi.js <out.json>`) and P06 on `task/P06` (read only): the generic routes
  `PATCH/PUT/DELETE /api/v1/config/{pointer}`, `POST /api/v1/config/commit?confirm=`, `/commit/confirm`, `/rollback/{rev}`, `/state/**`,
  auth (`Authorization: ApiKey …` — automation uses API keys, not the refresh cookie), problem+json errors with `errors[]{pointer,message}`
- `packages/api-client` (generated TS client — the pattern to mirror); `packages/schema` JSON Schema per domain (for typed resources)
- `docs/decisions/LOG.md` D-046 (write-only secrets: `passwordHash`, secret refs — clients must treat them as write-only/sensitive), D-051

## Scope — build exactly this
1. **Python SDK** `sdk/python/` (package `vrx`): generated models/operations from the OpenAPI file (`openapi-python-client` or
   `datamodel-code-generator` if already available via pip in a venv under the worktree — never system-wide installs), plus a thin hand-written
   `VrxSession` (API key, TLS verify on by default, candidate edit → commit with confirm → confirm/rollback helpers, problem+json → typed exception
   with pointer). `pytest` against a mocked HTTP layer + one live run against your slot's API (P06 e2e pattern).
2. **Terraform provider** `sdk/terraform/` (Go, `terraform-plugin-framework`, own Go module): provider config (url, api_key, insecure=false);
   resource `vrx_config` (pointer + JSON value, commits on apply with confirm/confirm-back, import by pointer); typed `vrx_interface` generated
   from the `interfaces` JSON Schema; data source `vrx_state`. Sensitive attributes for secret refs. Acceptance tests with `TF_ACC` only against
   your slot's API; unit tests with `httptest`. No registry publishing.
3. **Ansible collection** `sdk/ansible/` (`vrx.appliance`): module `vrx_config` (state present/absent at a pointer, check-mode = diff via the
   candidate, commit with confirm), `vrx_commit`, `vrx_facts` (state endpoints); uses the Python SDK; `ansible-test sanity`/units only if
   `ansible-core` is installable into the worktree venv.
4. **Generation script** `sdk/gen.sh`: exports OpenAPI from the API build, regenerates the Python models and the Terraform schema; CI check
   that generated output is clean (hook into `tools/ci.sh` only if it is fast; otherwise document as a manual gate).
5. **Docs**: `docs/user/system/sdk-terraform-ansible.md` — one worked example each (add a loopback + address, commit with confirm).

Files you own: `sdk/**`, `docs/user/system/sdk-terraform-ansible.md`, `test/topology/sdk-terraform-ansible/**`. Shared files: none (no web UI; the
OpenAPI document is P06's output — missing operation ids or schemas → `docs/status/tasks/F-sdk-terraform-ansible-questions.md`).

## Acceptance (paste the evidence)
- [ ] Python: live run creates a prefixed loopback + address via candidate → commit, reads it from `/state/interfaces`, rolls back (pasted)
- [ ] Terraform: `terraform plan/apply/destroy` with `vrx_config` against the slot API (or `TF_ACC` test output); second `plan` shows no diff
- [ ] Ansible (if reached): `ansible-playbook --check --diff` then apply of the same change (pasted), or listed as left over
- [ ] Secrets: a secret ref / password hash never appears in plan output, state files committed to fixtures, or SDK logs
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
RESTCONF/NETCONF/YANG (F-restconf-yang); the CLI (P13); changing API routes or OpenAPI generation (P06 — ask via questions); publishing to PyPI,
Terraform Registry or Ansible Galaxy; typed resources for every domain (only interfaces now); a Go/TS SDK beyond `packages/api-client`.

## Open questions to surface, not to decide silently
Terraform resource granularity: one generic pointer resource vs typed per domain (default: generic + generated typed where JSON Schema is simple).
Whether each Terraform apply should commit (default) or batch into one candidate per run (needs a session/lock story from P06).
