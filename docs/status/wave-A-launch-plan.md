# Wave A launch plan (prep-waveA critic, 2026-09-24; main@f5b801d, P08@task/P08)
**Gate:** spawn only after P08 (fix round F1–F6) **and TD-2** have merged, one at a time with `pnpm gen` after each. TD-2 owns `apps/api/**`, which
overlaps every feature's `apps/api/src/features/<slug>/**` and TD-4. Then commit this pack and the anchor commit (§5), then spawn batch 1.

## 1. Critic findings (envelopes in docs/status/tasks/ fixed in place)
- **Overlaps:** glob intersection over 21 ownership sets (17 feature envelopes, TD-4, TD-5/TD-6 running, TD-2) → 46 hits, all TD-2's `apps/api/**`;
  no other pair overlaps. Dep-chained shared files are hotspots (ED `desired/nat.go`/`rpc_nat44_ed*.go`/natTabs → EI; F-vrf's routes controller → P12).
  Latent: P10 (prio 6, ready after P08) owns `deploy/debian/` ⊃ P11's `deploy/debian/vrx-strongswan/**`.
- **Added:** A5 event sink for F-neighbors-ra and F-object-model (`agent/events.go` bus unexported; `subsystems.Env` has no publish hook). P12 W2 nav (the
  "routing tab registry by F-vrf-static-ecmp" does not exist). F-loopback D-105 loop16000–16383 rule. F-vlan-qinq D-105 F1 `items[].config` meaning.
  F-rpf-adl-pbr stale number-collision note removed.
- **Wave-B envelopes:** `contract/<id>` branch → contract commits on the task branch. "First to land creates" shells/sink → manager-seeded (parallel
  add/add otherwise). Secret channel = decision-policy #4.
- **CI line (18 envelopes):** the requested `flock … vrx-ci.lock` contradicts D-106 (serialising whole gates rejected; golangci-lint serializes itself
  since fc0fe68) → ngfw-46's TD-5 form `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main`, plus "ports 3000/8080/9101 = tools/app".
- **Daemons:** one owner each, all may run at once (slot test instances, D-089): P11 strongswan, P12 frr, F-kea kea, F-unbound unbound+chrony+rsyslog.
- **Host needs:** nothing NIC-gated, nothing parked; scope cuts only. P11 needs network (apt-get download + strongSwan tarball; RF-2's debs were lost in the
  09:47 reboot); kernel-vpp was tested on 5.9.x only, stock is 6.0.4 → 4 h feasibility checkpoint, else PENDING-network. P12: BGP between frrtest netns
  first; the FRR→VPP FIB proof waits for the linux-nl design (root netns; startup.conf change is handover-gated). auto_sdl fake-only (session layer).
  No DAD auto-remove. nsim/auto_sdl/globals host tests only in manager windows.
- **Deps:** the board matches the prompts (F-acl/F-host-acl ← F-object-model, F-loopback ← F-bridge-l2, EI ← ED, F-rpf ← TD-3).
  **Add P12 ← F-vrf-static-ecmp** (ListRoutes, /state/routes, viaFrr selector). Soft only: P11 ← F-tunnels/P12, F-acl ← F-rpf.

## 2. Batch 1 (slots 1–11; 12 = CI)
| slot | task | daemon-owner | why |
|---|---|---|---|
| 1 | F-vlan-qinq | none | first W5 toucher (drawer hunk), small → merge first |
| 2 | TD-5 (running) → P11 | strongswan | P11 is longest (24 h), heads P11→F-pki/F-ikev2→F-ra-vpn |
| 3 | F-vrf-static-ecmp | none | first A4 toucher, owns api actions, gates P12 → merge second |
| 4 | F-object-model | none | gates F-acl + F-host-acl-nftables |
| 5 | F-nat44-ed-sessions | none | gates EI, DET44, HA |
| 6 | TD-6 (running) → F-kea-dhcp-relay | kea | TD-6 before any real --apply (D-103) |
| 7 | TD-4 | none | prio 3, 5 h, TD-2 follow-up (TD-2's slot) |
| 8 | F-bridge-l2 | none | gates F-loopback + F-tunnels; needs `<ANSWER>` |
| 9 / 10 / 11 | F-neighbors-ra / F-rpf-adl-pbr / F-bonding | none | wave-A T1 (F-rpf owns the V23(a) fix) |

## 3. Batch 2 queue (first free slot, deps merged, prio 3 before 4)
F-acl → F-host-acl-nftables → F-nat44-ei-64-66-nptv6 → F-loopback-bvi-gso-lldp-span → P11 (if not started) → P12 (frr) →
F-kea-dhcp-relay (if not started) → F-unbound-chrony-syslog → F-wireguard. The next three need envelopes first: F-tunnels, F-det44 (may run beside EI),
P10 (must exclude vrx-strongswan).

## 4. Pairs that must not run together
- TD-2 × apps/api features · P12 × F-vrf-static-ecmp (dep) · P10 × P11 (deploy/debian) · ED × EI · F-bridge-l2 × F-loopback/F-tunnels · F-object-model × F-acl/F-host-acl
- P11 × P12 host phases: no `lcp default netns` globals window while P11 runs IKE over its fixture LCP pair (linux_nl is in the root netns).
- F-acl 100k-rule × F-vrf-static-ecmp 100k-prefix steps: never at the same time on the shared VPP (D-064); each in a manager window.
- Unseeded parallel creators: P11 × F-wireguard (vpn shell), F-kea × F-unbound (services shell), sink users × each other in agent.go.
- Semantic unions at merge: `Domains["services"|"acl"|"vpn"]` and duplicate `BUILT_DOMAINS` entries.

## 5. Manager actions before launch
1. **Commit the pack onto main.** Base is e7c6803. `F-bonding.md` and `F-loopback-bvi-gso-lldp-span.md` conflict with main's 70c5918 → take prep's
   side (it already has those fixes). F-vrf and F-neighbors-ra merge cleanly. Drop prep's `tech-debt/TD-5.md` + `TD-5.envelope.md` (stale vs D-105).
2. **Anchor commit (hotspots §4.2)**, also seeding the `Env` event-publish + Resync hooks (A5), `subsystems.SlotIDRange()`, the vpn/services page shells
   and the "Feature RPCs" heading in proto.md.
3. **Board:** set `files_owned` of the 17 feature rows to the envelope globs (4 empty, 13 stale `project_*.go`), plus TD-4's hunks; run `tools/board.py`.
4. **LOG, with options:** (a) P12 +F-vrf-static-ecmp: hard dep / soft + hotspot / merge order. (b) PENDING-secret-channel (#4): API push in a
   non-persisted field / agent pull RPC / resolver in the agent; park only the E2E secret steps (P11, F-wireguard, P12 passwordRef, F-unbound TLS).
   (c) F-bridge-l2 `<ANSWER>`: root key `l2` (PENDING #1) / per-member leaves + a named container (recommended). (d) Renderer stage: singleton
   descriptor per renderer (default) / service.go stage. (e) Confirm the §2 proposed numbers. (f) CI line per D-106. (g) V23(a) → F-rpf-adl-pbr.
5. **At spawn:** fill `<SLOT>/<BASE>/<STARTED>/<ANSWER>`. Before P11, check that the apt mirror and download.strongswan.org are reachable. If
   `sdk/gen.sh --check` joins ci.sh (D-093), add `sdk/**/_generated` to C7 first, or every route-adding gate goes red.
