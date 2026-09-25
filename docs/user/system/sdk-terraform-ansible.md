# Automation: Python SDK and Terraform provider

VRX is automated through its REST API (`/api/v1`). Two clients ship in the repository under `sdk/`; both are
**generated from the API's OpenAPI document** so a new configuration domain or field needs no client code:

| client | where | what |
|---|---|---|
| Python SDK (`vrx`) | `sdk/python` | every API operation as a method (generated), `VrxSession` for candidate → commit with confirm → confirm / rollback, typed errors with the offending JSON pointer |
| Terraform provider | `sdk/terraform` | `vrx_config` (any JSON value at any pointer), `vrx_interface` (typed, generated from the interfaces schema), data source `vrx_state` |
| Ansible collection | — | not built yet (see "Not available yet") |

Nothing is published to PyPI or the Terraform Registry; both are used from the repository.

## How changes are applied

Every change follows the appliance's transaction model:

1. edit the **candidate** (`PUT/PATCH/DELETE /api/v1/config/<pointer>`),
2. **commit with confirmation** (`POST /api/v1/config/commit?confirm=<seconds>`) — the change is live but provisional,
3. check that the API still answers, then **confirm** (`POST /api/v1/config/commit/confirm`).

If the client loses the appliance after step 2 (e.g. you changed the management interface), it never confirms and the
appliance **reverts on its own** when the window ends. A validation error at any step leaves the running configuration
untouched and the clients discard the candidate.

**The candidate is per user, not per API key.** All keys of one user share one candidate (D-093). The clients refuse
to start when the candidate already holds uncommitted changes. They also refuse to commit when a change they did not
make appears during their run, and they only ever discard a candidate that holds nothing but their own changes.
Two runs under the same user still get in each other's way (one of them fails), so **give every pipeline its own
service user** (with its own API key) and do not use that user interactively. Clear a leftover candidate with
`POST /api/v1/config/discard`.

**Sync state.** Both clients check `GET /api/v1/state/system` before editing and again before confirming. If the
running configuration and the data plane are not `in-sync` (`unknown` after a lost agent answer, `degraded` after a
partial apply), or if a confirmed commit is still pending, they stop without changing anything. The API reconciles on
its own; retry afterwards. Override with `allow_unsynced` only when you know why. If a commit's answer is lost
(`502`/`504`, "running-unknown"), the outcome is **unknown**: the API may still promote the change. Refresh or re-read
before retrying.

**Stored but not enforced.** When the agent build does not implement a domain yet, a commit touching that domain is
stored in the running configuration but not applied to the data plane. The API reports this as status
`partially-applied`/`not-applied` with `notApplied: [domains]` (P06). The SDK then warns (`NotEnforcedWarning`) or,
with `require_enforced=True`, raises `NotEnforced`. The Terraform provider reports a warning, or an error with
`fail_on_not_applied = true`.

**Not confirmed.** If the post-commit check fails, the change reverts at the deadline, but the candidate still holds
the edit. Terraform recognises its own leftover edit and re-uses it on the next run. With the SDK, call
`discard()` first.

## Credentials

Automation uses **API keys**, not passwords: create one in the web UI or with
`POST /api/v1/auth/api-keys {"name": "ci", "role": "operator", "expiresInDays": 90, "current": "<your password>"}` —
the key is shown once. From a login session (`Authorization: Bearer …`) `current` is required: it is your current
password (over https only; a few checks per minute per account, then 429; a wrong one counts toward the login lockout,
D-100). A request made with an existing API key (`Authorization: ApiKey …`) sends no `current`: it is refused with 400
(`current-not-allowed-with-api-key`, D-124), never checked. CLI: `vrx api-key create ci role operator expires 90 file ci.key`
asks for the current password without echo (or reads `--password-file`).
The role caps what the key can do: `readonly` (GET only), `operator` (configuration, not users/AAA/secrets),
`admin` (everything). Keep the key in a file or in the environment (`VRX_API_KEY`), never in code or in a Terraform
file. Use `https://`: plain `http://` is refused for anything but a loopback address unless you set
`allow_http=True` / `allow_http = true` (the key would travel in clear). TLS certificates are verified by default;
`verify=False` / `insecure = true` exist for lab boxes with self-signed certificates only, and `ca_file` trusts your own CA.

**Secrets.** Write-only members (today: `management.users[].passwordHash`) are never returned by the API. The SDK never
logs them (nor the API key) and the Terraform provider only accepts them through a *write-only* attribute, so they
never appear in a plan or in the state file. Pre-shared keys, private keys and certificates are not configuration
values at all: store them with `POST /api/v1/secrets` (admin; `VrxSession.put_secret()`) and reference them by name
(`psk/site-a`).

## Python: add a loopback with an address, commit with confirm

```python
import vrx

s = vrx.VrxSession("https://vrx-a.example.net", api_key_file="/etc/vrx/automation.key")

with s.transaction(confirm=60, comment="add loop10") as tx:          # candidate must be clean
    tx.set("/interfaces/loop10", {"enabled": True, "ipv4": ["192.0.2.10/32"]})
    print(tx.diff())                                                  # what will be committed
print(tx.result["status"], tx.result["revision"]["id"])              # 'confirmed', 42

for item in s.state("interfaces")["items"]:                          # live state from the data plane
    if item["name"] == "loop10":
        print(item["config"]["ipv4"])

s.rollback(41, confirm=60); s.confirm()                              # back to the revision before
```

Leaving the `with` block commits with `?confirm=60`, checks that `/state/system` answers `in-sync`, and confirms;
pass `check=` to use your own test (e.g. ping a next hop through the new interface). An exception inside the block
discards the candidate. Errors are typed:

```python
try:
    with s.transaction() as tx:
        tx.merge("/interfaces/loop10", {"ipv4": ["10.999.0.1/24"]})
except vrx.ValidationError as e:        # 400 problem+json
    print(e.pointer, e.errors[0].message)   # /interfaces/loop10/ipv4/0  Invalid IPv4 range
```

Other exceptions: `Unauthorized` (401), `Forbidden` (403, role too low), `NotFound`, `Conflict` (409, `e.lock` names
the lock owner), `CommitFailed` (422, `e.results`), `Unavailable` (502/503/504, `e.sync` says whether the running
state is known), `ConfirmError` (the commit was not confirmed and will revert), `TransportError`.
Every API operation also exists as a generated method named after its operationId
(`s.config_revisions(limit=10)`, `s.state_routes(vrf="default")`, `s.auth_create_api_key({...})`).
Set `log_bodies=True` and the `vrx` logger to DEBUG to see request bodies — write-only members are shown as
`<redacted>`, credentials never.

Install into a virtualenv from the repository (no runtime dependencies): `pip install ./sdk/python`.

## Terraform: the same loopback

The provider is not in a registry; build it and tell Terraform where it is (`~/.terraformrc`):

```sh
go -C sdk/terraform build -o "$HOME/.terraform.d/vrx/terraform-provider-vrx"
```
```hcl
provider_installation {
  dev_overrides { "registry.terraform.io/vrx/vrx" = "/home/me/.terraform.d/vrx" }
  direct {}
}
```

```hcl
terraform {
  required_providers { vrx = { source = "vrx/vrx" } }
}

provider "vrx" {
  url             = "https://vrx-a.example.net"   # or VRX_URL
  # api_key       — prefer the VRX_API_KEY environment variable
  confirm_timeout = 60                            # seconds; 0 = commit without confirmation
  # fail_on_not_applied = true                    # stored-but-not-enforced commits become errors (default: warning)
  # allow_unsynced      = false                   # default: refuse to edit while sync is unknown/degraded
}

# typed: attributes generated from the interfaces JSON Schema (defaults filled in like the API does)
resource "vrx_interface" "loop10" {
  name    = "loop10"
  enabled = true
  ipv4    = ["192.0.2.10/32"]
}

# generic: any JSON value at any pointer — every schema domain, no provider change needed.
# /system/hostname always exists on an appliance: import it first (terraform import vrx_config.hostname /system/hostname)
resource "vrx_config" "hostname" {
  pointer = "/system/hostname"          # canonical form: leading "/", no trailing "/"
  value   = jsonencode("edge-1")
}

# users (admin key only): the hash goes through the write-only attribute (never in plan or state) and is matched to
# the user BY USERNAME — never by position. /management/users exists by default: import it first.
resource "vrx_config" "users" {
  pointer                 = "/management/users"
  value                   = jsonencode([{ username = "admin", role = "admin" }, { username = "ops", role = "operator" }])
  sensitive_value         = jsonencode([{ username = "ops", passwordHash = var.ops_password_hash }])
  sensitive_value_version = 1          # bump to send a new hash
}

data "vrx_state" "interfaces" { path = "interfaces" }
output "loop10_state" {
  value = [for i in jsondecode(data.vrx_state.interfaces.json).items : i if i.name == "loop10"]
}
```

`terraform apply` makes **one confirmed commit per resource change**. The provider serialises them, and each commit
is a revision you can roll back to. A second `terraform plan` shows no changes. Drift of a `vrx_config` is measured
against the node **as the API stored it after the last apply**, so:
- members the API fills in as schema defaults are not drift;
- members changed **or added** out of band are drift, and the plan shows that the apply will remove them (the node is
  replaced as a whole);
- rewriting `value` with different formatting shows as an in-place update that commits nothing.

`vrx_interface` refuses to update an interface that has members its generated schema does not know (it would
delete them); rebuild the provider with `sdk/gen.sh`, or manage that node with `vrx_config`. `revision` is the
revision of the last commit made by the resource, or 0 after an import or a no-op apply.
Import existing configuration with the pointer or interface name:
`terraform import vrx_config.hostname /system/hostname`, `terraform import vrx_interface.loop10 loop10`.
Creating a resource over a node that already exists is refused with that hint. `sensitive_value` is a write-only
attribute and needs Terraform ≥ 1.11.

## Not available yet

- **Ansible collection** (`vrx.appliance`: `vrx_config`, `vrx_commit`, `vrx_facts`) — cut from this release; the
  Python SDK is its intended base. Until then, call the SDK from `ansible.builtin.script` or use the `uri` module
  against the API.
- Typed Terraform resources exist for interfaces only; every other domain is covered by `vrx_config`.
- One Terraform run = several commits (one per resource); batching a whole run into one candidate needs a session
  concept in the API.

## For developers

- `sdk/gen.sh [--check]` rebuilds the OpenAPI document from `apps/api` and regenerates
  `sdk/python/vrx/_generated/*` and `sdk/terraform/internal/provider/zz_*_gen.go`; `--check` fails when the committed
  files are stale. `sdk/test.sh` runs the unit gate (pytest against a mocked HTTP layer; `go test` against an
  `httptest` fake API through the plugin protocol). Neither is part of `tools/ci.sh` yet.
- Live runs against a real API + agent + VPP on a test slot:
  `eval "$(tools/lab env <slot>)"; test/topology/sdk-terraform-ansible/live.sh run sdk/python/.venv/bin/pytest -s sdk/python/tests/test_live.py`
  and `… live.sh run go -C sdk/terraform test -count=1 -v -run TestLive ./internal/provider`.
- The terraform CLI is not installed on the build host; the provider's tests drive it through the Terraform plugin
  protocol the way Terraform core does (`internal/tfharness`: validate, plan with core's proposed-new-state rule,
  apply with the post-apply consistency check, refresh, import).

CLI equivalent: the VRX CLI (P13) exposes the same candidate/commit/confirm operations.
