# Scheduler: tier-3 validators and the VPP→daemon stage

Package `apps/agent/internal/scheduler` (TD-13, audit ARCH-02, D-125). Code: `validator.go`, `stage.go`; the
contract is also in the package comment (`descriptor.go`).

## Why

Before TD-13 a daemon checked its configuration only inside its own `Create` (D-109 d). By then the VPP objects of
the same transaction had already changed, so a configuration the daemon refused cost a VPP write plus a rollback.
DryRun, the commit engine's tier-3 validation (`docs/contracts/proto.md` §3), ran no daemon checker at all.

## The pipeline

```
plan: desired checks → Retrieve → diff → topological order (ties: stage, registration order, key)
  → validate: Validator.Validate for every Create/Update of a descriptor that implements it
  → apply: deletes (reverse order), then creates/updates → verify → (rollback = exact reverse of the journal)
```

The scheduler validates in `Scheduler.Plan`, which the DryRun RPC uses, and in every `Apply`, resync and confirm
revert. Validation runs after planning and before the first operation, deletes included. A rejection is a plan
`Issue`. DryRun reports it (`ok = false`, no plan). Apply answers **FAILED** with nothing touched: no VPP call, no
daemon write, no rollback. A plan with issues keeps no operation.

`Scheduler.PlanWith(ctx, desired, scope, PlanOptions{SkipValidators: true})` plans without the validators. It is for
TD-9's periodic drift check only. That check asks whether the running state matches the stored desired state, not
whether a daemon would accept it. A rejection would also hide the drifted operations behind one issue. DryRun and
Apply always validate.

## `Validator` (optional descriptor extension)

```go
type Validator interface {
	Validate(ctx context.Context, key Key, value proto.Message, view ReadOnlyView) error
}
```

- `value` is exactly what Create or Update would receive (the Normalizer's output), passed as a copy.
- `view` (`ReadOnlyView`: `Get(key)`, `List(descriptor)`) is the state after the transaction. It holds the
  desired objects plus the actual objects the transaction keeps (out of scope, observe-only). Objects it deletes
  are absent. Values are copies and Meta is never exposed. A daemon whose file is built from several objects
  renders the whole file from `view.List(<its descriptor>)`.
- Called for: every Create and Update, including an Update that turns into a recreate. Not called for: deletes,
  unchanged objects, and dependents that a recreate cascade re-creates with their current value.

A validator must keep this contract:

| rule | why |
|---|---|
| **Read only.** No side effect outside a private temp dir it removes again (`renderers.Stage`). No daemon file written, no reload or restart, no VPP call, no ownership claim. | A plan may run a validator any number of times: DryRun, Apply, and the retry without dynamic sources. |
| **Never call back into the Scheduler** (`Plan`, `Retrieve`, `Apply`). Use the view. | `Plan` holds the read lock and `Apply` the write lock. A call back deadlocks behind a waiting Apply, or stalls until the deadline and becomes a false finding. |
| **Bounded by ctx.** The call gets a deadline: `Scheduler.ValidateTimeout`, where 0 means `DefaultValidateTimeout` (30 s) and a value below 0 means only the transaction's ctx. Start checkers with `renderers.Command{Timeout}` or `exec.CommandContext`. | The scheduler stops waiting at the deadline. The timeout is a finding (`validator did not return within 30s`), and nothing is applied. |
| **Safe for concurrent use** with itself, with `Retrieve`, and, once abandoned at its deadline, with the next transaction's operations. | `Plan` holds only the read side of the scheduler lock. |
| **Panics** are recovered on the validator's goroutine. | A panic is a finding (`validator panicked`). Its value and stack go to the agent log only. |
| **Name the leaf:** `scheduler.InvalidAt(pointer, err)` when the value carries an RFC 6901 pointer (for example a rendered rule's `pointer`). | The finding points at that leaf instead of the whole object. |
| **No secrets in the error.** A validator masks every plaintext it resolved from a reference (`rfkit.Redactor.Error`, the strongSwan and FRR `toolMessage`). The scheduler cannot know those plaintexts. | The scheduler masks the value's secret references and caps the text at 2 KiB. It masks every string with the D-051 form `psk\|key\|cert\|password\|token/<name>`, and every string field named `*_ref` (a malformed reference too; in a `google.protobuf.Struct` the keys are field names). By D-040 nothing else carries a secret across the API↔agent boundary. Object names, descriptions and BGP communities stay readable. The panic text is masked the same way before it is logged. |

### What a finding looks like

The scheduler turns a finding into `Issue{Key, Code: CodeInvalid, Rule: "agent.validator", Pointer, Message}`. The
message is `validator: <redacted error text>`. The agent maps the finding to a `ValidationIssue`, in the DryRun
report and in `ApplyResponse.validation` of a FAILED Apply:

```
{ "pointer": "/interfaces/loop701/description",            ← InvalidAt's pointer, else the object's own pointer
  "message": "interface.loopback/loop701: validator: checker: line 3: bad description",
  "severity": "ISSUE_SEVERITY_ERROR", "rule": "agent.validator" }
```

`ApplyResponse.results` carries the key with code `INVALID`. No proto field was added, because `rule` is a free
string (proto.md §3). The new stable rule id is `agent.validator`.

## Stages (`Stager`, optional)

```go
func (*Descriptor) Stage() scheduler.Stage { return scheduler.StageDaemon } // default: StageVPP
```

The stage breaks ties in the dependency sort. It is the first tie-breaker of the topological order, before the
registration order and the key:

- when a VPP-stage and a daemon-stage create or update are both ready, the VPP one goes first, so a daemon object
  that no VPP object waits for runs after every VPP object ready by then;
- deletes run in the reverse order, so daemon deletes go first when they are ready together.

It is a greedy tie-break, not a phase split. A real dependency always wins: a VPP object that depends on a daemon
object is created after it. A daemon object that such a VPP object waits for therefore runs early, and another
ready daemon object may run before that VPP dependent. The rollback undoes the journal in the exact reverse of what
ran. DryRun's `plan` lists the operations in the same order.

## Adoption recipe (a daemon descriptor implements Validator and declares StageDaemon)

Every renderer on `main` already has `Render(ctx, desired)` plus `Validate(ctx, files)`, which runs the checker on a
staged copy. Adopting TD-13 therefore means adding two methods to the singleton descriptor (D-109 d) and changing
nothing else. `Create` keeps its own check as defence in depth. The Kea example, checked against
`task/F-kea-dhcp-relay` in a scratch tree (`docs/status/tasks/TD-13.md` §4):

```go
// Stage implements scheduler.Stager (TD-13): Kea is configured after the VPP objects of its transaction.
func (*Descriptor) Stage() scheduler.Stage { return scheduler.StageDaemon }

// Validate implements scheduler.Validator (TD-13): kea-dhcp<N> -t on a staged copy — nothing written or reloaded.
func (d *Descriptor) Validate(ctx context.Context, _ scheduler.Key, value proto.Message, _ scheduler.ReadOnlyView) error {
	in, ok := value.(*vrxv1.DesiredState)
	if !ok {
		return fmt.Errorf("%w: %s value is %T, want *vrx.v1.DesiredState", ErrInvalid, d.Name(), value)
	}
	files, err := d.r.RenderFamily(in, d.family)
	if err != nil {
		return err
	}
	return d.r.Validate(ctx, files)
}
```

| feature | descriptor | `Validate` = | notes |
|---|---|---|---|
| F-kea-dhcp-relay | `kea.dhcp4/vrx`, `kea.dhcp6/vrx` | `RenderFamily(in, family)` → `r.Validate` (`kea-dhcp<N> -t <staged>`, `ip netns exec` on a slot) | Render is pure (subnet ids from the loaded assignment). |
| F-unbound-chrony-syslog | `unbound.config/vrx` (+ chrony, rsyslog alike) | `r.Render(ctx, in)` → `r.Validate` (`unbound-checkconf <staged>`; chrony `chronyd -p -f <staged>`, rsyslog `rsyslogd -N1 -f <staged>`) | Never call the `prepare` hook, because it creates directories. |
| F-host-acl-nftables | `host-acl.nftables/vrx` | `r.Render(ctx, v)` → `r.Validate` (`nft -c -f <staged>`) | Do not take the descriptor's apply mutex for longer than the render. Never touch the store. |
| P12 (FRR) | its frr descriptor(s) | `r.Render(ctx, doc)` → `r.Validate` (`vtysh -C -f <staged>`); `frr-reload.py --test` only for a diff | If several objects build one `frr.conf`, render it from `view.List(...)`. Password references are resolved and redacted by the renderer. |
| P11 (strongSwan) | its swanctl descriptor(s) | `r.Render(ctx, doc)` → `r.Validate` (`swanctl --load-all --noprompt --file <staged> --uri <scratch charon>`) | Only the scratch charon is loaded (start_action rewritten to none), never the live one. PSKs are redacted by the renderer. |
| F-snmp | `snmpd` descriptor | `r.Render` → `r.Validate` (`snmpd -C -c <staged check copy>`) | The check instance runs inside the staging dir and is stopped by its PID, never the live snmpd. The community plaintext is resolved from `Community.secret_ref` and masked by the renderer (`r.red`); the scheduler masks the reference. |

**Test pattern.** Copy `validator_test.go`'s `fakeDaemon`: a failing Validate must leave the fake VPP with no write
call and the answer FAILED. Also test the adopter's own `Validate` with a recording runner. It must make exactly one
checker call, on a staged path, and nothing else: no `config-set`, no reload, no file written. Go cannot enforce side
effect freedom, so this test plus review is the mechanism.

## Limits

- Validation runs inside the transaction lock (Apply) and inside the read lock (Plan). A slow checker delays the
  transaction by up to its bound, per Create/Update of that descriptor.
- A checker validates syntax and what it can see offline. Semantic conflicts that only the live daemon sees at
  commit time still fail in Create and roll back, as before.
- **Up to 3 checker runs per commit** for a changed daemon object: the commit engine's DryRun, Apply's plan, and
  Create's own check (defence in depth). An Apply whose dynamic source was to blame runs again and adds more. Kea
  costs about 100 ms per run. P11's scratch charon is the costly one; its review measures it.
- TD-9's drift check (`CheckDrift`, a Plan every 5 minutes) must use `PlanWith(…, PlanOptions{SkipValidators: true})`.
  That is a condition for TD-9's rebase (TD-13 review M2).
- A validator that ignores ctx is abandoned, and its goroutine lives until it returns. There is no counter yet
  (tech debt, review L9).
