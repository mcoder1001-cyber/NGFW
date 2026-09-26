# renderers/nftables — the host firewall (F-host-acl-nftables, D-057)

The single owner of the host firewall: `acl.host`, `acl.hostAttachments` and `acl.hostSettings` become ONE nftables
table, `table inet vrx` (test slots: `table inet vrx_<prefix>` inside their own network namespace). Nothing else is
ever touched — no `flush ruleset`, no other table — so P10's static base policy and foreign tables survive. F-hardening-lite
consumes this renderer. Mapping table and semantics: `docs/agent/renderers/nftables.md`; operator view:
`docs/user/firewall/host-acl-nftables.md`.

## Files

| file | what |
|---|---|
| `build.go` | `Build(Input) (*HostTable, []Issue)`: pure; expands objects (`internal/objects`), plans chains/rules/sets, reports issues at JSON pointers |
| `lockout.go` | the anti-lockout check (probe simulation through the input chains) |
| `renderer.go` | `renderers.Renderer`: `Render` (text, every token re-checked), `Validate` (`nft -c -f <staged>`), `Apply` (one `nft -f`), `Retrieve` |
| `parse.go` | `nft -j list table` → `KernelTable` (counters) → `HostTable` (normalised) |
| `descriptor.go` | scheduler descriptor `host-acl.nftables/vrx` + the store of the value last applied |
| `state.go` | `HostAclState` answer, runtime registry (`RuntimeFor(stateDir, owner)`) |
| `paths.go`, `runner.go` | modes (`apply` / `netns` / `check`), product and test paths; the netns runner (setns on a locked thread) |
| `nftables_model.proto` | the descriptor value (`HostTable`, `Set`, `Chain`, `Rule`), agent-internal |
| `nftest/` | test-only harness: slot namespaces + veth pair, sockets inside a namespace, root-netns check |
| `testdata/` | golden renderings (checked with `nft -c`) and real `nft -j` output of two of them (nft 1.1.6) |

## Lifecycle (renderer.go contract)

- **Render** — pure: the value (`HostTable`) → one file (`<state dir>/host-acl-<owner>.nft`, 0600):
  `add table inet vrx` · `delete table inet vrx` · `table inet vrx { sets…; chains… }` (the last part only when there is
  a chain). Every token is checked again (`HostTable.Validate`, `checkRuleText`) because a value can come from the store.
- **Validate** — `nft -c -f` on a staged copy (`renderers.Stage`); `-c` never changes the ruleset.
- **Apply** — snapshot, atomic write, `nft -f <file>`: the kernel applies the whole file as one transaction or nothing;
  on failure the previous file is restored (the kernel still has the previous table). Mode `check` writes the file only.
- **Retrieve** — `nft -j list table inet <t>`: a missing table is `nil`, not an error.

## Descriptor (decision (a))

The agent core runs descriptors only (`service.applyLocked`), so the renderer rides on one singleton descriptor
registered under `Domains["acl"]`: Create/Update = Render → Validate → Apply → store; Delete = remove the table → forget
the store; Retrieve = the kernel table plus, from the store, the configuration and the rule annotations. Rules pair by
their comment `vrx:<id>/<n>:<hash8>` (hash = sha256 of the rendered rule text), so nft's own re-printing of a rule never
matters and a lost, edited or foreign rule shows as a difference (→ Update). The value carries the applied
configuration (from the store, D-063: real agent state) so `Retrieve(acl)` equals the committed `acl.host*`.

## Running processes

`nftables.Binaries()` = `/usr/sbin/nft` only (`internal/renderers/ALLOWLIST.md`). Argv: `nft -c -f <staged>`,
`nft -f <file>`, `nft -j list table inet <table>`. Mode `netns` runs each call on a fresh goroutine that locks its OS
thread and `setns(2)`s into `/run/netns/<ns>` (never `ip netns exec`); the thread is never unlocked. Only the product owner
(`vrx`) may load into the root namespace (`Paths.Validate`).

## Tests

- Unit (`go test ./internal/renderers/nftables/`): golden files (`-update` rewrites; each is checked with `nft -c` when
  run as root — a check is allowed in the root netns of the shared host), hostile input (quote/brace/newline/NUL/…
  in descriptions, list, object and interface names; hand-made hostile values), the anti-lockout matrix, a round trip
  through real `nft -j` output (Retrieve == desired), descriptor lifecycle with a `RecordingRunner`, path safety.
- Integration (`VRX_INTEGRATION=1`, lab lock shared): `nftables_integration_test.go` in `ns-<prefix>-hacl` with a veth
  peer — packets, counters, update, rollback, lost-table re-render, a foreign table untouched, removal; `nftest` fails the
  test if the root netns `nft list tables` changed.

## Generated code

`nftables_model.pb.go` is generated from `nftables_model.proto` (never hand-edited):

```sh
cd apps/agent/internal/renderers/nftables
PATH=$PATH:$HOME/go/bin protoc -I . -I ../../../../../packages/proto \
  -I "$(go env GOMODCACHE)/github.com/bufbuild/protocompile@v0.14.1/wellknownimports" \
  --go_out=. --go_opt=paths=source_relative nftables_model.proto
```
