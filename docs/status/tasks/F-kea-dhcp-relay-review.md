# F-kea-dhcp-relay — review

Reviewer session, 2026-09-25. Branch `task/F-kea-dhcp-relay` @ `ed26523f` (main merged in at `0736530e`; merge base
`b5e74c08`). Read: envelope, prompt, status + questions (Q1–Q11), contract note, 00-CONTEXT, 01-architecture,
shared-host-rules, LOG D-063/D-079/D-109/D-119/D-125/D-128/D-129/D-132/D-133/D-134, TD-11b's ownership guard
(`dfkit/persist`, `subsystems/stores.go`), wave-A-hotspots, wave-BC-numbers, and the full diff (86 files).
The host test was not re-run, because slot 2 is being cleaned.

## Verdict: **APPROVE WITH CHANGES**

The architecture is sound. The Kea singletons are real Retrieve with drift detection, not an echo. The relay is VRF-scoped and fails
closed. Node reaches VPP and Kea only through the agent, and the UI takes its types from the schema and the api-client. The
contract is additive and has no number collisions. The host evidence is strong. None of the findings needs a redesign.
Fix M1–M3 in a short fix round, with a focused re-verify and no full re-review. M4 and the merge notes are for the merger.
Q5 is a real **H** defect on **main**, not on this branch, and needs its own row.

## Tests run by the reviewer (this tree, `ed26523f`)

```
$ cd apps/agent && go test -race -count=1 ./internal/renderers/kea/... ./internal/descriptors/dhcp/... ./internal/subsystems/... ./internal/agent/... ./internal/desired/...
ok  	ngfw/agent/internal/renderers/kea	4.046s
ok  	ngfw/agent/internal/descriptors/dhcp	1.282s
ok  	ngfw/agent/internal/subsystems	6.871s
ok  	ngfw/agent/internal/agent	13.686s
ok  	ngfw/agent/internal/desired	1.269s
$ pnpm --filter @ngfw/web test          (after `turbo run test` built the workspace deps; the bare filter fails to resolve @ngfw/schema without dist/)
 ✓ src/domains/services/kea-dhcp-relay/DhcpPage.test.tsx (8 tests)
 Test Files  15 passed (15)      Tests  107 passed (107)
$ packages/schema: vitest run src/semantic/kea-dhcp-relay.test.ts     Tests  10 passed (10)
$ apps/api: vitest run (unit)                                           Test Files 10 passed · Tests 96 passed
$ apps/api e2e, dhcp module only, on an isolated reviewer DB (VRX_TEST_PREFIX=rvkea, created and dropped by global-setup; no VPP):
 ✓ test/e2e/kea-dhcp-relay.e2e.test.ts (6 tests) 3425ms
 drop database vrx_rvkea · drop role vrx_rvkea · ok nothing named vrx_rvkea remains
$ git -C /root/ngfw merge-tree --write-tree main task/F-kea-dhcp-relay
e35a379c55873a0cde2356c688adc65ba4064b16        (exit 0: no conflicts with main)
```
`turbo run test` also ran `gen`, and the worktree stayed clean, so the committed generated files match the generator. The
`dist/` and `openapi.json` build outputs were removed afterwards.

Probe for M1, run through a `go test -overlay` file so the tree was never touched: `Status(4)` over a copy of the packaged,
commented `/etc/kea/kea-dhcp4.conf`, with the daemon not running.
```
Status(4) = running=false active=true actionRequired="start" err="kea: configuration: invalid character '/' looking for beginning of value" subnets=0
Retrieve = 0 kvs, err=<nil>
```

## 1. Architecture

- **One singleton per daemon (`kea.dhcp4/vrx`, `kea.dhcp6/vrx`) is sound under D-109(d).**
  - It keeps D-109(d)'s point: a descriptor wraps the renderer, and `service.go` gets no stage.
  - Splitting by daemon isolates each family. Each Apply carries one file, so a failed `config-set` rolls back only its own
    family (`renderer.go` Apply loops over the families present).
  - Both descriptors share one `*Renderer`, whose `ids` state is keyed by family.
- **Kea Value = the render input, embedded in the config, then ConfigDrift: correct, and not an echo (D-063).**
  - Retrieve reads the input back from the daemon's own state: `config-get`, or, when the daemon is stopped, the file it
    will load (`descriptor.go:166-191`).
  - It re-renders that input and returns it only when `ConfigDrift` finds the daemon running exactly that rendering.
    Otherwise it returns a `structpb.Struct` drift value, which can never equal a desired Value.
  - Every input change changes the embedded blob and therefore the config, so an input can never differ from what the
    daemon runs without being seen.
  - Accepted limit: ConfigDrift is a subset match (RF-3), so a key injected by someone else that the rendering lacks goes
    unseen.
  - `TestDescriptorLifecycle` covers drift, converge, the stopped daemon, delete to idle, and foreign/commented configs.
- **Relay names in an agent-local record (`dhcp.relay`): restart-safe.**
  - The record is a 0600 JSON file in the state dir (`relay.go:107-178`). Its Retrieve reports each server only when
    `dhcp_proxy_dump` has it with the record's rx VRF, server VRF and src (`relay.go:262-303`).
  - The host run shows the lost proxy recreated and the record updated at +0.41 s.
  - A lost store only re-creates records; there is no VPP operation.
  - One partial echo: a *disabled* relay's record is pure store content. That is acceptable, because a disabled relay has
    nothing on the data plane.
- **Node never talks to VPP or Kea.**
  - `KeaDhcpRelayController` uses only `AgentClient` (`dhcpLeases`, `retrieve(['services'])`) and the datastore.
  - The agent drives Kea only over its unix sockets (D-079), with no kea-ctrl-agent.
- **One schema for three consumers.**
  - The UI types come from `@ngfw/schema` (`DhcpServer`, `DhcpSubnet`, `DhcpRelay`, `RootConfig`) and the generated
    `paths` (`model.ts:1-45`).
  - The forms come from the generated domain JSON Schema (`domainSchemas.services`).
  - No DTO is written by hand.
- **Foreign Kea configs are never touched (Retrieve, Delete).**
  - A config without an embedded input is never reported, so it is never deleted or rewritten; the probe above shows
    `Retrieve = 0`.
  - The `Status` path does not follow the same rule (see M1).
  - In product mode, once a server exists in the document, the agent does write `/etc/kea` by design (see Q6).

## 2. Safety and security

- **Q7: `ip netns exec` in the product binary.** Rated **L**. There is no privilege gain.
  - **How test mode is chosen:** only by the agent's environment, `VRX_KEA_MODE=test` (`subsystems/kea.go:83-99`).
    - Neither the API nor the config document can reach it. Only root can set a unit's environment.
    - An unknown value refuses to start.
  - **The command:** fixed argv `ip netns exec <ns> /usr/sbin/kea-dhcp{4,6} -t <staged file>` (`renderer.go:245-250`).
    - The netns comes from env and is validated against `^[a-z0-9][a-z0-9-]{0,31}$` (`paths.go:125`), so it cannot start
      with `-`.
    - The binary is fixed, and the staged path is created by the agent.
    - `SystemRunner` checks the allowlist and runs `exec.CommandContext` with no shell (`helpers_exec.go:136-157`).
  - **The residual risks are misconfiguration and documentation:**
    - A product unit that inherits `VRX_KEA_MODE=test` would silently write its configs to `/run/vrx-test/...`.
    - RF-3's invariant says `ip` is never in the product. `ALLOWLIST.md:69` still reads "test-only, never in
      kea.Binaries()". That stays literally true, but the product binary now builds such a runner (`kea.go:143-144`) and
      the document does not say so.
  - See the recommendation under Q7.
- **Hostile input into the Kea JSON is safe.**
  - Configs are marshalled with `encoding/json`, and RF-3's checks are unchanged: `check.go` handles names, hostnames,
    MAC/DUID, printable option data and escaped descriptions.
  - The new `user-context.vrx` carries base64 protobuf plus a name list; `encoding/json` escapes the names, and the list
    is never read back.
  - The schema has no client classes and this branch adds none.
  - Zod and Go validate the same things, with one gap: the length of DHCPv4 option data (L2).
- **File ownership and mode are correct.**
  - Rendered configs are 0640, `_kea:_kea` in the product and root in test (evidence: `-rw-r-----`).
  - The socket directory must be 0750 or stricter; `checkSocket` refuses group-write and any access for others.
  - The relay store is 0600, in a 0750 directory.
  - Lease files are Kea's own memfile, which the agent never reads. Leases come only through `lease*-get-page`.
- **V19 pre-flight and per-interface guard.**
  - Both run before the veths go up and before any dhclient packet (`dhcp_test.go:597-610`).
  - Minor ordering gap: VPP's own DHCP client is committed earlier (`dhcp_test.go:541`, see L4).
- **The relay fails closed via TD-8.**
  - `w.IDRange()` returns `NoIDs()` = {1,0} together with `ErrNoIDRange`, so the scope predicate is always false: relays
    are refused and nothing is retrieved.
  - `IDs.All` returns nil, which means the product owns every VRF (`kea.go:73-81`).
  - This is correct, but no test covers it (M3).
- **D-132: 30 s polling plus a Refresh button.** Both are present (`queries.ts:18`, `DhcpPage`).
  - `/state/dhcp/relays` runs `Retrieve(['services'])`: about six `dhcp_proxy_dump` calls, `ip_table_dump`, and Kea
    `config-get`.
  - These are bounded by the number of relays and VRFs, and none is a FIB or session walk.
  - They are not serialised against each other: `sched.Retrieve` takes the RLock and excludes only transactions. That is
    acceptable at this size (L5).
  - The lease path is the real cost (M2).

## 3. Contract

- **The single commit is additive: `71071f2d contract(schema,proto)`.**
  - It adds `rpc DhcpLeases` and six new `Dhcp*` messages, numbered from 1, in the `// ----- F-kea-dhcp-relay -----`
    section, plus proto.md §11.
  - It adds 4 rules in the task's own file plus one spread in `semantic/index.ts`.
  - Nothing is renamed or reshaped. `1f956f10 contract(schema)` is prettier only.
- **DhcpRelay 9–10 are unused.** This is intended: they are allocated only for option-82 / remote-id, which was not built
  (wave-BC-numbers "Batch-2 follow-ons").
  - Keep them in the ledger for the follow-on, next free DhcpRelay 11.
  - Do **not** add a proto `reserved 9, 10;`, because that would block the follow-on from using them.
- **No collisions.** I scanned the proto additions of every in-flight `task/*` branch:
  - no other branch adds a `Dhcp*` message, a `DhcpLeases` rpc, or a `DhcpRelay`/`DhcpServer` field;
  - F-unbound-chrony-syslog uses SyslogTarget 9 and ActionRequest 7, both allocated;
  - F-host-acl-nftables and F-acl use AclConfig 8 / 7;
  - F-loopback uses ServicesConfig 9 `nsim`;
  - this branch adds no field to `ServicesConfig`.

## 4. Claims (TD-11b, D-133)

- **`dhcp.client` is claim-first** (`client.go:115-128`): `tg.ClaimFirst` → add, then `claim.Adopt` on INVALID_VALUE, or
  `claim.Undo` on failure.
  - `TestClientClaimsBeforeVPPWrite` fails on the old order, because it asserts 0 `dhcp_client_config` calls when the
    claim fails.
  - Q9 is resolved: the code already uses TD-11b's helper.
- **The five `RecordsNoOwnership()` declarations are each correct:**
  - `kea.dhcp4/6`: ownership is the embedded input in the daemon's own config, with no claim or boot store and no VPP
    object.
  - `dhcp.proxy` / `dhcp.proxy-vss`: **no claims are needed.**
    - The relay allocates no ID. It uses the rx VRF's table ID, which the VRF family allocates and owns by the name
      `<owner>:<vrf>`.
    - The ownership predicate "rx VRF ∈ `Wiring.IDRange()`" is a pure function of (VPP state, the agent's env range). A
      restarted agent computes the same set, so nothing needs to be persisted.
    - Claims are for ownership that cannot be derived from VPP plus config: untagged interfaces, or routes, which have no
      tag field and so need the owner table.
    - The only way to lose ownership is to change the agent's range between restarts, and that affects every
      range-scoped family the same way.
    - `CheckPersistent` would have nothing to check.
  - `dhcp.relay`: the store holds metadata, not ownership, as argued in §1. The file store is used anyway.

## 5. Q5: P08/core deletes a DHCP-leased address. **Confirmed (H, on main; not this branch's defect)**

- `main:apps/agent/internal/descriptors/core/ifaddr.go:208-246`: `InterfaceAddrDescriptor.Retrieve` reports **every**
  address that `ip_address_dump` returns on every **tagged** (agent-created) interface (`:216-219`, `in.ID != ""`). VPP's
  DHCPv4 client installs its lease with the ordinary interface-address call, so the lease is among them.
- `main:apps/agent/internal/desired/interfaces.go:226-244` never desires that address, and the branch's new rule forbids
  a static IPv4 next to `dhcpClient`. The next reconcile of `interfaces` (any commit, or a resync) therefore deletes the
  lease with `sw_interface_add_del_address is_add=0`.
- `:320-321` assembles the lease into `ipv4`, so `/state/drift` shows it as well.
- The default route the client installs is safe: `core/route.go:162-172` treats FIB source DHCP as client-programmed.
- **Scope:**
  - On main, untagged physical NICs are skipped entirely (`ifaddr.go:217`).
  - On `task/TD-11c` (`ifaddr.go:307-348`, `core.go:266-274`) an untagged NIC's address is reported only when claimed.
    A lease is never claimed, so that case stays safe.
  - The bug therefore hits DHCP on **sub-interfaces** (for example a VLAN WAN), **af_packet** (the lab rig) and
    loopbacks.
  - The topology test never reaches BOUND (no server on the client's link), so it cannot show the bug.
- **Fix:**
  - The interface-ip Retrieve skips, per `sw_if_index`, the prefix that `dhcp_client_dump` reports as that interface's
    lease (`lease.host_address/mask_width`).
  - Inject it as an optional `core.Env` hook, for example `LeaseAddrs func(ctx) (map[uint32]string, error)`, wired in
    `subsystems.go` from DF-8's `ClientDescriptor.Leases`. Core then does not import the dhcp plugin.
  - Test: a fake-VPP BOUND lease on a tagged sub-interface, then reconcile deletes nothing and drift is empty.
- **Owner:** a new TD row, core (P08's successor), **merged after TD-11c**, because TD-11c rewrites `ifaddr.go` (220
  lines). It must land before any release or TEST-traffic row puts a DHCP client on a sub-interface.
- Until then, the user guide's Example 3 needs a known-issue line (L7).

## 6. Evidence and tests

- **The host evidence is convincing and complete for the acceptance list:**
  - dhclient got DISCOVER→ACK of 10.2.1.100 through the relay.
  - Option 82 was echoed: circuit-id = sw_if_index 1, link-selection 10.2.1.1.
  - Counters rose on both rig interfaces.
  - The lease appears in `/state/dhcp/leases` with a MAC filter.
  - `show dhcp proxy` shows `2001 10.2.2.1 2001,10.2.2.2`.
  - Restart after simulated loss of proxy, client and Kea config: back in 0.20–0.41 s, with no config API call, and a
    second lease through the recovered path.
  - Rollback: relays `[]`, proxy and client gone, `config-get subnet4=[]` without user-context.
  - 400 problem+json with pointers `…/pools/0/start|end`.
  - NRestarts 1→1, and no trace command anywhere.
- **Gaps:**
  - The VPP client is only ever in DISCOVER, so the BOUND fields of `/state/interfaces/{name}/dhcp-client` (address,
    router, dns) are unit-tested only.
  - `subsystems/kea.go` has no unit test (M3).

## Findings

| # | sev | where | finding | fix |
|---|---|---|---|---|
| M1 | M | `renderers/kea/status.go:51-65`, `renderer.go:354-367` | `Status` treats a config without an embedded input as managed. On any stock box, or this dev host's product stack, the packaged commented `/etc/kea/kea-dhcp{4,6}.conf` does not parse, so `active()` returns true. The result is `active=true`, `actionRequired="start"` and an `error`. The Servers tab then shows two Error chips and the "start its service" warning, with no DHCP configured (probe above). This contradicts decision 6 ("foreign configs are never reported"). | In `Status`, a config for which `EmbeddedInput` returns ok=false is unmanaged: not active, no action, no subnets, no error. Add a unit test with the commented file, both with the daemon stopped and running. |
| M2 | M | `status.go:209-243`, `rpc_kea.go:101-127`, `web …/queries.ts:32-37` | Every `DhcpLeases` call reads up to `MaxLeases` = 100 000 leases **per family** from Kea: 100 `lease*-get-page` round trips per family. That includes the status poll (`pageSize: 1`) every 30 s, on top of the Leases tab's own 30 s poll: two full reads per viewer per 30 s, unserialised. Kea handles control commands on the same thread that answers DHCP. | No contract change needed: add a singleflight plus a short TTL cache (about 10 s) of the raw lease list per family in the agent. Optionally let the status query reuse the Leases tab's response. |
| M3 | M | `subsystems/kea.go:66-147` (no test) | There are no tests for mode parsing (a bad value refuses to start), IFMAP validation, or the **fail-closed relay scope**. The scope relies on `IDRange()` returning a non-nil `NoIDs()` together with the error (`:73-81`). A harmless-looking refactor to `if err != nil { rng = nil }` would give a slot agent every VRF of the shared VPP, and it would then delete the other slots' and the product's relays. | Add `TestRegisterKeaRelayScope`: with a zero `IDs`, a proxy in table 5000 is not retrieved and Create is refused. With `IDs.All`, it is retrieved. Also add a mode table test. |
| M4 | M (merge) | `subsystems.go` (const + Domains), `desired/kea.go:24-45`, `nav.test.ts` | Collisions with F-unbound-chrony-syslog that git auto-merges without a conflict but that break the build: both define `Services = "services"` and a `Domains[Services]` key; both declare `desired.ServicesImplemented` (theirs in `dns_services.go:17`); both run an unsupported-field emitter (`ServicesUnsupported` and their `unsupported()`), so each would flag the other's sub-key; and `'services'` appears twice in `nav.test.ts`. | The second lander folds into the first lander's const, key, map and emitter (its own `init()` adds `ServicesImplemented["dns"/"ntp"]`) and drops its duplicate `'services'`. Mechanical (D-134). |
| L1 | L | `packages/schema/src/semantic/kea-dhcp-relay.ts:7-8` | The doc says the agent re-checks all four rules. `reservation-outside-pools` and `dhcp-client-no-static` have no Go counterpart. | Fix the comment, or add the in-pool check to the renderer's `reservation()`. |
| L2 | L | `services.ts:106` vs `renderers/kea/check.go:38,122-131` | Zod allows DHCPv4 option data of up to 1024 characters; the renderer caps it at 255. The API accepts the value, and the commit then fails at apply, is rolled back, and gives no pointer. This mismatch comes from RF-3/P02c. | Add an additive rule to this file: DHCPv4 `options[i].data` ≤ 255, with a pointer. |
| L3 | L | `descriptors/dhcp/relay.go:138-151`, `:263-266` | `FileRelayStore.save` does no fsync of the file or the directory (P08 N7 pattern). An unreadable file fails `Retrieve`, and with it every transaction that touches `services`. | fsync as P08 N7 does. On unmarshal error, log and treat the store as empty, since the records are re-creatable metadata. |
| L4 | L | `test/topology/kea-dhcp-relay/dhcp_test.go:541` vs `:600-603` | VPP's DHCP client on `host-w2c0` is committed in `commit` and sends DISCOVERs (VPP TX) before the V19 guard runs in `relay`. The risk is low: the veths are down and the interface is sanitized at creation (D-095). | Run the pre-flight and guard right after `kea-base`, before the `kea-dhcp` commit. |
| L5 | L | `kea-dhcp-relay.controller.ts:217-248` | `/state/dhcp/relays` runs a whole-domain `Retrieve(['services'])`. After F-unbound merges, the same poll also runs unbound, chrony and syslog reads. | Accept for now. Revisit if the services domain grows. |
| L6 | L | `renderers/kea/control.go:73-75` | Kea control commands inside Retrieve (under the scheduler lock) time out after 30 s, so a hung daemon stalls transactions for up to 30 s per family. | TD-9 (bounded calls) scope. Note only. |
| L7 | L | `docs/user/services/kea-dhcp-relay.md:111-120` | Example 3 (WAN DHCP client) does not mention Q5. | Add a known-issue line (sub-interface / af_packet) until the Q5 row merges. |
| L8 | L | `renderers/kea/input.go:88-98` | The embedded input copies all of `DhcpServer` into `/etc/kea` and into `config-get` output. A future secret (DDNS/TSIG) would leak there. | When a secret field arrives, strip it from the input, and add a guard test. |
| L9 | L | `coretest/kea_dhcp_relay.go` (`dhcpModels sync.Map`) | The model state is keyed by `*VPP` and never removed (test-only leak). | At the TD-23 rebase, keep the model state inside the extension closure. |

## Merge notes (for the merger)

- **Main:** `merge-tree` is clean (tree `e35a379c`).
- **D-125 gate "merge after TD-13":**
  - TD-13 is `ready` and has not started. Its branch equals main.
  - I recommend **lifting the gate for this branch**, making `kea.dhcp4/6` TD-13's first Validator consumer, and
    writing that into TD-13's acceptance. Reasons:
    - Kea is registered before the relay families, so inside `services` no VPP write precedes Kea validation.
    - Only earlier domains' VPP changes in the same transaction are exposed, and the scheduler rollback covers them.
    - Holding a finished branch behind an unstarted 8 h row is the kind of waiting the standing orders ask to remove.
  - This is the manager's decision.
- **TD-23, which lands first:**
  - Delete the `v.installKeaDHCPRelay()` line in `coretest/fakevpp.go:96`. Add
    `func init() { RegisterExtension("kea-dhcp-relay", (*VPP).installKeaDHCPRelay) }` to `coretest/kea_dhcp_relay.go`.
    No other extension models `dhcp_proxy_*`, so the collision guard stays quiet.
  - `fake-agent.ts`: `dhcpLeases: dhcpLeases(this)` is a unary feature-RPC line under its anchor. That is TD-23's kept
    pattern, and there is no Action, so **no** `registerActionHandler` is needed. The unanchored import line stays.
  - `merge-tree task/TD-23 task/F-kea-dhcp-relay` is clean.
- **In-flight branches:**
  - The only real textual conflicts, in files both branches change, are:
    - generated files (`apps/agent/gen`, `packages/proto/gen/ts`, `api-client schema.d.ts`, `apps/cli operations_gen.go`):
      regenerate;
    - `agent/service_test.go` against F-unbound-chrony-syslog, F-rpf-adl-pbr and F-loopback: the same assertion change
      with different comments, plus the `canonicalDoc` `"services"` line. Recompute it from the merged assemblers.
  - The `docs/vpp-code-track.md` conflicts that pairwise merge-trees show come from older bases and disappear after a
    rebase onto main.
  - Plus M4.
- **Squash (D-112):** the branch touches `packages/proto`, `packages/schema`, `apps/agent/gen` and `api-client/src/generated`.
  The single squashed subject must start with `contract(`, for example
  `contract(schema,proto,api-client): F-kea-dhcp-relay — Kea singletons, VPP relay, DhcpLeases, DHCP UI`.

## Recommendations on the questions

- **Q2** (relay and Kea server in the same VRF): **allow**, as proposed. It is the lab topology and a coherent design.
  - Add one sentence to the guide: VPP relays the *whole* rx VRF, so clients in that VRF reach a local Kea only if the
    relay's server list points at it.
  - Whether DHCP reaches a linux-cp-bound Kea while the dhcp plugin handles bootps in VPP is **unverified**. Put it on
    P12's test list.
- **Q3** (P02c example against the one-VRF rule): the example's owner (the manager) sets `customer-a` to
  `"enabled": false`, which keeps the two-VRF shape in the example. Extend TD-22's `examples.test.ts` item so that every
  example runs through the full semantic registry. That would have caught this.
- **Q4**: agree. Do not project it now. When a DHCPv6 client leaf gets a number (Interface 23+ is next free), the UI
  shows "configured, state unknown" (write-only, D-063).
- **Q5**: confirmed; see §5. It is **H on main**, fixed by a new core TD row after TD-11c, and does not block this
  branch.
- **Q6**: agree. Two additions:
  - (a) Until P12 and TD-13, an enabled server fails at apply (`NoMapper`), with a rollback and no 400. The guide should
    say "refused at commit", not imply validation.
  - (b) This dev host's integrated stack (tools/app) runs the agent in product mode. A committed server, even a disabled
    one, is rendered into `/etc/kea/kea-dhcp4.conf`. Set `VRX_KEA_MODE=off` in tools/app until P12, so the shared host's
    `/etc/kea` stays untouched.
- **Q7**: keep the env-selected test mode. A `labtest` build tag is not needed, and it would make the host test exercise
  a different binary from the product. Harden it, all small:
  - (a) refuse `VRX_KEA_MODE=test` when the owner is the product owner (`vrx`) or `IDs.All`;
  - (b) log the mode at WARN when it is `test`;
  - (c) extend the `ALLOWLIST.md:69` row: "also the agent's `VRX_KEA_MODE=test` runner (`subsystems/kea.go`), fixed
    argv, never in product mode";
  - (d) a unit test that product mode's runner refuses `/usr/bin/ip`.
  Severity is L: no user input and no privilege gain; the risk is misconfiguration.
- **Q8**: correct. See M4 for how the second lander folds.
- **Q9**: resolved (`ClaimFirst` is in use).
- **Q10, Q11**: fine as described.
