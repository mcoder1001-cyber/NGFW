# DF-5 — Descriptors: ipsec, ikev2, wireguard (WBS D6.1, D6.3, D6.5)

Branch `task/DF-5`, worktree `/root/ngfw-wt/DF-5`, slot 4 (`w4`, ids 4000–4999, ports 204xx).
Worked in three passes: first pass (ipsec, ikev2, wireguard, D-063/D-065), a stall, and this
continue pass, which merged main and brought every descriptor up to the rules decided since
(D-069, D-071, D-074, D-076, D-080, D-082, D-087, D-089) and moved the host checks onto P05's real
reconciler with a restart simulation.

## What was built

Object types (16 descriptors + helpers), all under `apps/agent/internal/descriptors/`:

| Plugin | Descriptor | Retrieve | Ownership |
|---|---|---|---|
| ipsec | `ipsec.spd` | `ipsec_spds_dump` | ownership record (boot identity) + id range |
| ipsec | `ipsec.spd-interface` | `ipsec_spd_interface_dump` | record `<sw_if_index>/<spd_id>/<pool index>` |
| ipsec | `ipsec.spd-entry` | `ipsec_spd_dump` | through its SPD |
| ipsec | `ipsec.sa` | `ipsec_sa_v5_dump` (keys → references) | record `<spi>/<protocol>` + id range |
| ipsec | `ipsec.tunnel-protect` | `ipsec_tunnel_protect_dump` | tag of the tunnel interface |
| ipsec | `ipsec.itf` | `ipsec_itf_dump` | tag `<owner>:ipsec<N>` |
| ipsec | `ipsec.backend` | `ipsec_backend_dump` | VPP-global (D-071: owner setter / Require) |
| ipsec | `ipsec.async-mode` | write-only (`ErrRetrieveUnsupported`, D-063) | VPP-global |
| ikev2 | `ikev2.profile` (composite, 12 setters) | `ikev2_profile_dump` (PSK → reference) | name `<owner>-<name>` |
| ikev2 | `ikev2.responder-hostname` | write-only (not dumped) | applied-once record (D-076) |
| ikev2 | `ikev2.local-key` | write-only | VPP-global |
| ikev2 | `ikev2.sleep-interval` | `ikev2_get_sleep_interval` | VPP-global |
| ikev2 | `ikev2.liveness` | write-only | VPP-global |
| wireguard | `wireguard.interface` | `wireguard_interface_dump` (show_private_key never set) | tag `<owner>:wg<N>` |
| wireguard | `wireguard.peer` (+ `Events` → PeerEvent for StreamEvents) | `wireguard_peers_v2_dump` (PSK → reference) | tag of its wg interface |
| wireguard | `wireguard.async-mode` | write-only | VPP-global |

Helpers: `ikev2.SAs` (SA state, derived keys zeroed) + action helpers; `ipsec.SweepCharonOrphans` /
`ipsec.SweepAndAck` (D-089); `vpn` package (desired-state proto, secret references + Resolver,
D-069 interface resolution over DF-1's `iface.Table`, ownership records over `dfkit.BootStore`
bound to `vpp/bootid`, `vpn.Global` / `vpn.Require` for D-071); `vpn/vpntest` (host fixtures,
P05-based `Agent` for plans/applies, fake boot identity, globals lock).

Docs: `docs/agent/descriptors/{ipsec,ikev2,wireguard}.md` (object ↔ message tables, ownership,
secret contract for P11, VPP limitations, tests).

### This pass, rule by rule

* **D-069** logical interface names: every interface reference is resolved with DF-1's resolver
  (`vpn.DumpInterfaces` → `iface.Table.IndexByName`): our tag id, else an untagged interface's VPP
  name; another owner's interface → `ErrForeignInterface`; VPP's name of our own interface →
  `ErrNoInterface`. Retrieve reports logical names. Tunnel-protect and WireGuard peers require our
  own (tagged) interface (`ErrNotOurs` for untagged).
* **D-071** globals only in a globals-owner registration: `vpn.Global(owner, setter, getter)` —
  owner: setter with `DeleteOnAbsence()==false`; non-owner: `vpn.Require` (check via VPP's getter,
  never set; getter-less globals always `ErrNotGlobalsOwner`; Retrieve write-only). Packages take
  `WithGlobalsOwner`. **Claim rule:** tags for tagged objects; untagged objects (SPD, SA, SPD
  binding) only through an ownership record written after OUR successful add, never before it,
  existing objects never adopted (VPP refuses existing ids; Create fails). **Deletes by id/index
  re-verify identity** in the same call sequence (SA: id + SPI + record; SPD: record; policy: still
  in our SPD; binding: name still resolves to the recorded index + record; itf / wg interface / tunnel
  protect: index still carries our tag and id; wg peer: index still holds the same public key on
  the same interface).
* **D-074** a delete first checks the object exists (vanished → nil, no VPP call).
* **D-076** write-only Creates idempotent: async modes, local key, liveness verified idempotent in
  VPP's source; `ikev2_set_responder_hostname` is NOT (vec_dup leak + resets resolution per call)
  → applied-once record keyed by the boot identity; dropped when the profile is re-added.
* **D-080** boot identity via `apps/agent/internal/vpp/bootid` (through `dfkit.BootIdentity`); a
  VPP restart expires every record (unit tests simulate it with `vpntest.NewFakeBoot`).
* **D-089** charon orphan sweep + `AckRestart` only after a clean sweep.
* Dedupe keys from dumps (`dfkit.Dedupe` in every Retrieve); `vpn.ErrRetrieveUnsupported` is now
  P05's sentinel.
* **Restart simulation** on the host through P05 (below).

## How it was verified

All host runs on slot 4, one package at a time (D-087), `systemctl show vpp -p NRestarts` before
and after every run: 4 → 4 each time (the earlier restarts are the incidents of D-064/D-087).

### Unit tests (fake VPP)

```
$ go test -count=1 ./internal/descriptors/vpn/... ./internal/descriptors/ipsec/... ./internal/descriptors/ikev2/... ./internal/descriptors/wireguard/...
ok  	ngfw/agent/internal/descriptors/vpn	0.029s
?   	ngfw/agent/internal/descriptors/vpn/pb	[no test files]
?   	ngfw/agent/internal/descriptors/vpn/vpntest	[no test files]
ok  	ngfw/agent/internal/descriptors/ipsec	0.034s
ok  	ngfw/agent/internal/descriptors/ikev2	0.031s
ok  	ngfw/agent/internal/descriptors/wireguard	0.037s
$ golangci-lint run ./internal/descriptors/vpn/... ./internal/descriptors/ipsec/... ./internal/descriptors/ikev2/... ./internal/descriptors/wireguard/...
0 issues.
```

### Host checks through P05 (VRX_INTEGRATION=1, `eval "$(tools/lab env 4)"`)

`TestIpsecOnHost` (excerpt, prototext bodies elided):

```
apply: APPLIED {Created:10 Updated:0 Deleted:0 Unchanged:0 Failed:0 Reverted:0}
ipsec.spd: Retrieve == desired: spd_id: 4001
ipsec.spd-interface: Retrieve == desired: interface: "loop401"        (tagged loopback)
ipsec.spd-interface: Retrieve == desired: interface: "loop402"        (untagged loopback = physical NIC stand-in)
ipsec.spd-entry: Retrieve == desired: spd_id: 4001
ipsec.sa: Retrieve == desired: sad_id: 4001 … 4004
ipsec.tunnel-protect: Retrieve == desired: interface: "ipip4001"
ipsec.itf: Retrieve == desired: instance: 4001
SA swap plan: update ipsec.tunnel-protect/ipip4001
apply: APPLIED {Created:0 Updated:1 Deleted:0 Unchanged:9 Failed:0 Reverted:0}
agent 1, second apply: P05 plan of the same desired state (10 objects): 0 create, 0 update, 0 delete, 10 unchanged; write-only re-apply: []
agent restart (fresh agent, persisted records): P05 plan of the same desired state (10 objects): 0 create, 0 update, 0 delete, 10 unchanged; write-only re-apply: []
ipsec.spd: nothing retrieved (agent without our ownership records)
ipsec.sa: nothing retrieved (agent without our ownership records)
ipsec.spd-interface: nothing retrieved (agent without our ownership records)
ipsec.spd-entry: nothing retrieved (agent without our ownership records)
after loss (SA + NIC binding deleted via the API): plan create ipsec.spd-interface/loop402; create ipsec.sa/4002
apply: APPLIED {Created:2 Updated:0 Deleted:0 Unchanged:8 Failed:0 Reverted:0}
after re-creation: P05 plan of the same desired state (10 objects): 0 create, 0 update, 0 delete, 10 unchanged; write-only re-apply: []
ipsec.backend: ipsec_backend_dump returned no backend on VPP 26.06 — nothing to require
ipsec.async-mode (non-owner): ipsec.async-mode: not the globals owner: VPP-global settings are managed by the globals owner only (D-071); VPP has no getter, configure it on the globals owner
charon sweep: deleted SAs [4501], policies 1, in use []; AckRestart calls 1
after the charon sweep (our SAs untouched): P05 plan of the same desired state (10 objects): 0 create, 0 update, 0 delete, 10 unchanged; write-only re-apply: []
apply: APPLIED {Created:0 Updated:0 Deleted:10 Unchanged:0 Failed:0 Reverted:0}
ipsec.spd / spd-interface / spd-entry / sa / tunnel-protect / itf: nothing retrieved (after applying the empty desired state)
--- PASS: TestIpsecOnHost (0.17s)
```

`TestIkev2OnHost`:

```
ikev2.sleep-interval: VPP reports seconds: 2
ikev2.sleep-interval (non-owner): requirement satisfied without setting
ikev2.liveness (non-owner): refused … not the globals owner …
ikev2.local-key (non-owner): refused … not the globals owner …
apply: APPLIED {Created:3 Updated:0 Deleted:0 Unchanged:0 Failed:0 Reverted:0}
ikev2.profile: Retrieve == desired: name: "df5-psk"
ikev2.profile: Retrieve == desired: name: "df5-rsa"
ikev2.responder-hostname: applied; write-only (Retrieve: vpp has no dump for this object type)
update plan: update ikev2.profile/df5-psk
apply: APPLIED {Created:0 Updated:1 Deleted:0 Unchanged:2 Failed:0 Reverted:0}
agent 1, second apply: P05 plan of the same desired state (3 objects): 0 create, 0 update, 0 delete, 3 unchanged; write-only re-apply: []
agent restart (fresh agent, persisted records): P05 plan of the same desired state (3 objects): 0 create, 0 update, 0 delete, 2 unchanged; write-only re-apply: [create ikev2.responder-hostname/df5-rsa]
apply: APPLIED {Created:1 Updated:0 Deleted:0 Unchanged:2 Failed:0 Reverted:0}      (hostname: skipped in VPP by its applied-once record)
after loss (profile deleted via the API): plan create ikev2.profile/df5-psk
apply: APPLIED {Created:1 Updated:0 Deleted:0 Unchanged:2 Failed:0 Reverted:0}
after re-creation: P05 plan of the same desired state (3 objects): 0 create, 0 update, 0 delete, 3 unchanged; write-only re-apply: []
ikev2 SA state helper: 0 SAs for owner w4 (no peer)
apply: APPLIED {Created:0 Updated:0 Deleted:3 Unchanged:0 Failed:0 Reverted:0}
ikev2.profile: ikev2.profile/df5-psk gone
ikev2.profile: ikev2.profile/df5-rsa gone
--- PASS: TestIkev2OnHost (0.52s)
--- SKIP: TestIkev2GlobalsOwnerOnHost (VRX_DF5_GLOBALS=1 only: getter-less globals cannot be restored)
```

`TestWireguardOnHost`:

```
apply: APPLIED {Created:3 Updated:0 Deleted:0 Unchanged:0 Failed:0 Reverted:0}
wireguard.interface: Retrieve == desired: instance: 4001
wireguard.peer: Retrieve == desired: interface: "wg4001"   (×2, with and without PSK)
agent 1, second apply: P05 plan … 0 create, 0 update, 0 delete, 3 unchanged
wireguard peer events: subscribed (want_wireguard_peer_events ok), no status change without a real peer
agent restart (fresh agent): P05 plan of the same desired state (3 objects): 0 create, 0 update, 0 delete, 3 unchanged
after loss (peer removed via the API): plan create wireguard.peer/wg4001/kNb/3zQk6wVr9jYVwTZg+Qy5xtyxIV119TzJgOREvHM=
apply: APPLIED {Created:1 Updated:0 Deleted:0 Unchanged:2 Failed:0 Reverted:0}
after re-creation: P05 plan … 0 create, 0 update, 0 delete, 3 unchanged
wireguard.async-mode (non-owner): … not the globals owner … VPP has no getter …
apply: APPLIED {Created:0 Updated:0 Deleted:3 Unchanged:0 Failed:0 Reverted:0}
wireguard.interface / wireguard.peer: nothing retrieved after the empty desired state
--- PASS: TestWireguardOnHost (0.55s)
```

(the peer key id is the peer's **public** key, a test vector.)

### CLI evidence (read-only `vppctl show`, redacted, leak guard)

Captured with `VRX_DF5_PAUSE=10` while each test held its objects, then after the empty desired
state. Every line from a key / auth-data word onward is replaced by `<redacted>`; a leak guard
greps the capture for the raw and hex test vectors and for any 32-byte key in base64 / hex. Full
file: `/root/ngfw-wt/logs/DF-5-vppctl-evidence.txt`.

```
NRestarts before: 4
===== ipsec: objects held by TestIpsecOnHost =====
$ vppctl show ipsec sa | grep " sa 4[0-9][0-9][0-9] "
[0] sa 4003 (0xfa3) spi 5002 (0x0000138a) protocol:esp flags:[]
[1] sa 4004 (0xfa4) spi 5003 (0x0000138b) protocol:esp flags:[inbound ]
[3] sa 4502 (0x1196) spi 6502 (0x00001966) protocol:esp flags:[]            ← live "charon" SA, kept by the sweep
[4] sa 4001 (0xfa1) spi 5000 (0x00001388) protocol:esp flags:[inbound ]
[5] sa 4002 (0xfa2) spi 5001 (0x00001389) protocol:esp flags:[esn anti-replay tunnel udp-encap ]
$ vppctl show ipsec spd (blocks of spd 4001/4501)
spd 4001
 ip4-outbound:
   [0] priority 10 action bypass type ip4-outbound protocol any
     local addr range 10.4.1.0 - 10.4.1.255 port range 0 - 65535
     remote addr range 10.4.3.0 - 10.4.3.255 port range 0 - 65535
     packets 0 bytes 0
 (other sections empty)
spd 4501
 (all sections empty — the orphan's protect policy was swept)
$ vppctl show ipsec protect | ipip4001 block
ipip4001 flags:[none]
 output-sa:
  [0] sa 4003 (0xfa3) spi 5002 (0x0000138a) protocol:esp flags:[]
 input-sa:
  [4] sa 4001 (0xfa1) spi 5000 (0x00001388) protocol:esp flags:[inbound ]
  [1] sa 4004 (0xfa4) spi 5003 (0x0000138b) protocol:esp flags:[inbound ]
$ vppctl show interface | grep -E "ipsec4001|ipip4001|loop4"
ipip4001                          3     down         9000/0/0/0
ipsec4001                         26    down         9000/0/0/0
loop401                           24    down         9000/0/0/0
loop402                           4     down         9000/0/0/0
===== ipsec: after the empty desired state =====
(all four commands: no lines)
===== ikev2: objects held by TestIkev2OnHost =====
$ vppctl show ikev2 profile | profiles w4-*
profile w4-df5-psk
  auth-method shared-key <redacted>
  local id-type fqdn data w4-local.vrx.test
  remote id-type rfc822 data peer@w4.vrx.test
  local traffic-selector addr 10.4.6.0 - 10.4.6.255 port 0 - 65535 protocol 0
  remote traffic-selector addr 10.4.7.0 - 10.4.7.255 port 1000 - 2000 protocol 17
  protected tunnel loop403
  responder loop402 10.4.5.2
  udp-encap
  NAT-T disabled
  ipsec-over-udp port 20402
  ike-crypto-alg aes-cbc 256 ike-integ-alg hmac-sha2-256-128 ike-dh modp-2048
  esp-crypto-alg aes-cbc 128 esp-integ-alg sha1-96
  lifetime 3600 jitter 10 handover 5 maxdata 1073741824
profile w4-df5-rsa
  auth-method rsa-sig auth data <redacted>
  local id-type ip4-addr data 10.4.5.1
  responder loop402 0.0.0.0 peer.w4.vrx.test
  lifetime 0 jitter 0 handover 0 maxdata 0
===== ikev2: after the empty desired state =====
(no lines)
===== wireguard: objects held by TestWireguardOnHost =====
$ vppctl show wireguard interface | wg4001
[0] wg4001 src:10.4.8.1 port:20410 private-key <redacted>
$ vppctl show wireguard peer | peers on wg4001
[0] endpoint:[10.4.8.1:20410->10.4.8.2:20411] wg4001 keep-alive:25 flags: 0, api-clients count: 0
  adj:
  pre-shared key <redacted>
  public key <redacted>
  allowed-ips: 10.4.10.0/24 10.4.9.0/24
[1] endpoint:[10.4.8.1:20410->0.0.0.0:0] wg4001 keep-alive:0 flags: 0, api-clients count: 0
  adj:
  public key <redacted>
  allowed-ips: 10.4.11.0/24
===== wireguard: after the empty desired state =====
(no lines)
NRestarts after: 4
leak guard: clean
--- PASS: TestIpsecOnHost (10.16s)
--- PASS: TestIkev2OnHost (10.45s)
--- PASS: TestWireguardOnHost (10.56s)
```

### Acceptance greps

```
$ grep -rn "vppctl\|exec.Command" internal/descriptors/{ipsec,ikev2,wireguard,vpn}
(no output)
$ grep -rniE "VRX_TEST_PSK|private_key" <the three host test logs>
ev-wireguard.log:4:        private_key:  "x25519:bJoxz03VtfsctmGeCV0KWoQdMXg/9MJy1BL/zwYc0xM="
```

The one hit is the field name of the WireGuard interface's reference, whose value is the
interface's **public** key (the `x25519:` reference form) — no material.

### CI gate

`tools/ci.sh --base main` on `ba1c6a0` (the commit after it only pastes this output). Excerpt of
`/root/ngfw-wt/logs/DF-5-ci-final.log`:

```
== contract guard: HEAD vs main ==
no contract files changed in the 18 commit(s) of HEAD since main (58fe694)
== forbidden patterns (+ gitleaks) ==
ok: gitleaks — no leaks found
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   1m28s
  forbidden patterns (+ gitleaks)                    0m03s
  lint · typecheck · unit tests · build (turbo)   1m29s
  apps/agent: make lint test build                   0m27s
  test/ Go modules, unit mode (test/integration/smoke)   0m01s
  mode quick · wall time 3m32s · logs /root/ngfw-wt/logs/ci/DF-5-20260924-044958-2805165

CI GATE PASSED
```

## Out of scope / left undone

* Globals-owner setters of getter-less globals on the host (ikev2 liveness/local key, async modes):
  unit-tested on the fake; `TestIkev2GlobalsOwnerOnHost` runs only with `VRX_DF5_GLOBALS=1` in a
  manager window (their previous values cannot be read back and restored, §7). Async modes: no
  worker threads on the host.
* Charon SPD / bypass-policy cleanup and charon id allocation (Q10, P11).
* Packet-level ESP / IKE negotiation / WireGuard handshakes (P11, F-*), wiring of `Register` and the
  persisted record store into the agent (P05/P08, Q12), PeerEvents → StreamEvents (Q8).

## Open questions (docs/status/tasks/DF-5-questions.md)

Q1 (gitleaks false positive, own commit recreated again per D-067), Q2 (IKEv2 id truncated by govpp
— V-track), Q3 (WireGuard src_ip dependency key), Q5 (two providers of `interface/ipsec<N>` /
`interface/wg<N>`), Q6 (proto placement, P03b), Q8 (PeerEvents wiring), Q9 (hostname responder after
resolution, F-*), **Q10** (charon SPDs/ids, P11), **Q11** (secret reference format vs D-051 —
recommend keyed digest), **Q12** (persisted record store for P05/P08). Q4 and Q7 closed.

## Decisions taken (with options)

| # | Decision | Options | Why |
|---|---|---|---|
| D-DF5-1 | Untagged VPN objects (SPD, SA, SPD binding) are owned through records in the owner's `dfkit.BootStore`, written after our own add, bound to the D-080 boot identity; SA record includes SPI/protocol, binding record includes sw_if_index + spd_id + SPD pool index | (a) id range only (first pass) (b) iface ClaimStore (c) BootStore records | (a) adopts foreign/stale objects after a VPP restart (D-071); (b) is keyed by interface name only; (c) also fixes the SPD pool-index restart limitation (Q7) |
| D-DF5-2 | SPD bindings are ours only with a record, also on our tagged interfaces | (a) tagged interface → binding ours (b) record always | charon's kernel-vpp may bind its own SPD to any interface; (b) never deletes it |
| D-DF5-3 | Tunnel-protect and WireGuard peers only on our tagged interfaces (`ErrNotOurs` for untagged) | (a) allow untagged with claims (b) tagged only | tunnel / wg interfaces are always created by some owner; an untagged one is not a NIC |
| D-DF5-4 | VPP-globals via `vpn.Global`: owner setter (never deleted on absence) / `vpn.Require` (getter check, write-only) | (a) df6-style generic singleton (b) small wrapper over the existing setters | (b) keeps the existing descriptors and their tests; same semantics as DF-6/DF-8 |
| D-DF5-5 | Responder hostname applied once per boot identity + value (D-076); the profile drops the record on add/delete | (a) re-apply every resync (b) applied-once record | VPP's setter leaks and resets resolution on every call |
| D-DF5-6 | Charon orphan sweep deletes SAs + their protect policies only; SAs used by a tunnel protection are reported; ack only after a clean sweep | (a) also delete charon SPDs (b) SAs/policies only | a fresh charon SPD cannot be told from a stale one (Q10) |
| D-DF5-7 | Host checks use P05's scheduler (`vpntest.Agent` + DF-1 alias) instead of a local diff helper | (a) keep the local helper (b) P05 | P05 is on main; the restart simulation must use the real reconciler |
| D-DF5-8 | Own commit `8fecca0` recreated (`f7154a9`) to remove a gitleaks false positive | (a) rewrite own unmerged commit (b) `.gitleaksignore` (c) red gate | D-067 precedent; (b) not my file |
