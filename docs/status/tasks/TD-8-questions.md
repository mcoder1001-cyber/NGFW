# TD-8: questions and findings for the manager

## Q1. `subsystems.go` is outside the owned files but needed two lines (the manager decides)
Scope item 4 says "SlotIDRange goes through Env", and `Env` is declared in `subsystems.go`. Two fields were added:
`Env.IDs IDScope`, and the unexported `Wiring.seams seamRegistry` that holds the dynamic sources and metrics collectors registered
through `seams.go`. There are 5 added lines, all in the struct declarations and none near a `wave-A:` anchor, so they merge line-locally
with the A1 feature lines. Option (a) accept them as seam hunks. Option (b) move `Env` into `seams.go` at merge. (a) is recommended.

## Q2. A new test file and a test seam in `agent.go`
The unit tests of the agent-side seams are in a new file, `apps/agent/internal/agent/seams_test.go`. No existing test file was
edited. To exercise `Start`'s real wiring against a fake VPP, `agent.go` gained
`var dialVPP = func(socket, opts) vppConn { return vpp.Dial(socket, opts) }`. Production behaviour is unchanged.

## Q3. Fail closed means "owns no id", not "refuses to start", while unset (decided; one line flips it)
`VRX_VPP_TABLE_BASE` and `VRX_VPP_ID_RANGE` both unset → `Config.IDs` is the zero `IDScope`, and `Wiring.IDRange()` returns
`ErrNoIDRange`. Start-up logs a warning. A family that allocates ids fails its registration, so once such a family is wired, the
agent refuses to start. A malformed value, both variables set, or `VRX_VPP_ID_RANGE` other than `all` refuses start-up now
(`Config.Validate`).
I did not refuse start-up on "unset" yet. Two launchers that I do not own start `vrx-agent` without either variable, and
refusing now would break them the moment TD-8 merges:
- `tools/app` (`start_svc agent env … VRX_OWNER=vrx …`): add `VRX_VPP_TABLE_BASE=13000` (§12 below).
- `test/topology/interfaces/interfaces_test.go` (`st.agentEnv` is a clean environment, run by `tools/ci.sh full`): pass
  `"VRX_VPP_TABLE_BASE="+os.Getenv("VRX_VPP_TABLE_BASE")` through. `live.sh` and `apps/api/test/integration/agent.int.test.ts`
  inherit the slot environment already.

With both changed, the eager form is one line in `ConfigFromEnv`: drop the `errors.Is(idsErr, subsystems.ErrNoIDRange)` exception.
The P10 packaged unit needs `VRX_VPP_ID_RANGE=all` in its EnvironmentFile. The manager decides when to flip it; I recommend doing so
before the first id-allocating family (F-acl, P11) merges.

## Q4. `SlotIDRange()` changed meaning: unset is now `ErrNoIDRange`, no longer nil ("every id")
This follows the envelope. W-seed's contract was "nil = the product agent owns every id". Nobody calls it yet. I grepped every
wave-A worktree for `SlotIDRange`, `.Publish(` and `RequestResync` and found no user outside the seams files. Features should
read the range with `w.IDRange()` (through Env), not from the environment.

## Q5. S1 design: merged into every transaction (decided; for LOG.md)
Options:
- (a) Merge the source's Desired and descriptors into every transaction (Apply, resync, revert, DryRun), plus its own sync scoped
  to its descriptors, under the txn lock. Chosen.
- (b) The source's descriptors are out of scope for config transactions, and only its own sync plans them.
- (c) Each loop writes VPP itself. This breaks "one writer", so it was rejected.

(b) looks more isolated, but the scheduler lets an out-of-scope object block the delete of what it depends on ("cannot delete: X
(not managed by this transaction) depends on it"). With (b), removing an interface that an LDP label route uses fails the user's
commit until the next FRR poll. With (a), `Desired(doc)` sees the post-transaction document, drops that route, and one
transaction deletes both in dependency order (tested).
The cost of (a) is that a failing dynamic object also rolls back the config transaction it rides in. A source with a key outside its
descriptors fails every transaction (`agent.dynamic-source`), which is a programming error caught by the feature's unit tests.

## F1 (finding, `tools/ci.sh`, not mine): the contract guard fails intermittently under `pipefail`
`do_contract_guard` runs `git log --format=%s "$mb..$TIP" | grep -qiE '^contract(\(|:|!)'` under `set -euo pipefail`.
`grep -q` exits at the first match, `git log` then gets SIGPIPE (141), and pipefail turns the match into "no contract commit".
On this branch (64 commits since main, first contract commit at line 13) it failed on 2 of 3 runs:
```
$ bash -c 'set -o pipefail; for i in $(seq 20); do git log --format=%s $mb..HEAD | grep -qiE "^contract(\(|:|!)"; echo -n "$? "; done'
0 141 141 141 0 0 141 141 0 141 141 0 141 0 0 0 0 0 0 141
$ … | grep -iE "^contract(\(|:|!)" >/dev/null   (5 runs)
0 0 0 0 0
```
Fix (owner P09/manager): `grep -iE … >/dev/null` (or `grep -c`) instead of `grep -q` in that pipeline. The same pattern may exist
elsewhere in `ci.sh`.
