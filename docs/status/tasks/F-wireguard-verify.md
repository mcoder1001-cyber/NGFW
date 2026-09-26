# F-wireguard verify — fix round 1 (focused on the review findings)

Scope: the review's findings only (`F-wireguard-review.md` @ `6b79e77a`: F1, F2, F3, F9, F10, F11, the Q2 UI default,
and CI), checked on `task/F-wireguard` @ `173e49b4`. Fix commits: `fbe995fd` (F1), `009722e7` (F2), `f6a12853` (F9 + UI
default), `f8a61e19` (docs), `173e49b4` (CI evidence). No host runs.

## Verdict: APPROVE

All required findings are fixed and tested. The remaining notes are low severity and do not block.

## F1 — routing loop: fixed (TS and Go agree, IPv6 included)

- **Schema rule.** `vpn.wireguard-route-loop` is in `packages/schema/src/semantic/wireguard.ts`. It collects the IP
  endpoints of *every* WireGuard interface, keyed by `underlayVrf`. For each interface with `routeAllowedIps`, it
  refuses any allowed IP that contains an endpoint whose underlay VRF equals this interface's `vrf`. The issue sits at
  `/vpn/wireguard/interfaces/<if>/peers/<p>/allowedIps/<i>`, so the commit gets a 400 problem+json with that pointer
  (the same pipeline as the duplicate-key e2e). `vrf` and `underlayVrf` both default to `'default'` in the schema
  (`_shared/primitives.ts:139,150`), so the equality test has no undefined-versus-`'default'` hole.
- **Go builder.** `desired/wireguard.go:87-109, 196-201, 247-254, 446-453` normalises through `vrfName` and uses
  `netip.Prefix.Contains` on the unmapped address. It raises an ERROR at the same pointer and drops the route.
- **Tests.** Schema tests cover same-VRF 0/0 (with the full message), the whole pipeline, routes off, another overlay
  VRF, and another interface's endpoint. Go test `TestWireguardRouteLoopRefused` covers the refusal in the default VRF
  and the projected route in VRF red.
- **My probes of the IPv6 and edge cases.** On the TS side I ran the built rule on a fresh `dist`. On the Go side I used
  `go test -overlay` with a scratch test file, so nothing was written to the tree. Both sides agree on every case:

| case | TS | Go |
|---|---|---|
| `::/0`, IPv6 endpoint, same VRF | refused | refused |
| `2001:db8:9::/48` covering the IPv6 endpoint | refused | refused |
| `::/0` with an IPv4 endpoint (different family) | allowed | allowed |
| `0.0.0.0/0` with explicit `vrf`/`underlayVrf` `default` | refused | refused |
| `/32` equal to the endpoint | refused | — |
| overlay `red` (or underlay `red`, overlay default) | allowed | allowed |

- **User doc.** It carries the loop paragraph, the three workarounds, the duplicate-route note, and the rule in the
  validation list.
- **Note (L, non-blocking).** A peer **without** a configured endpoint (a road warrior), with a routed `0.0.0.0/0` in
  `vrf == underlayVrf`, still loops at run time once VPP learns its endpoint. The rule cannot see a learned address.
  Server-side road-warrior peers normally carry `/32`s, so this is rare. A warning for "default-route allowed IP,
  no endpoint, same VRF" would close it. Add it to the follow-up list.

## F2 — the fixture stays test-only: fixed

- **`wireguard_fixture_guard_test.go`.** An untagged static AST scan of the package's non-test `.go` files. The only
  assignment to `wireguardFixture` must be in `wireguard_fixture.go`, and that file must begin with the build tag.
- **`wireguard_fixture_nil_test.go`.** Tagged `!vrxtestsecrets`: in a product build the hook is nil.
  - Together they cover both the moved-init case and the dropped-tag case. A declaration-with-initialiser in an
    untagged file is caught by the nil test.
  - Both pass. With `-tags vrxtestsecrets`, the static guard passes and the nil test is correctly excluded.
- **`tools/ci.sh:379` (one line, `do_forbidden` 4b).** A `git grep -Iw --untracked` that excludes `*_test.go`, the
  tagged fixture, `test/`, `docs/`, `*.md` and `ci.sh` itself.
  - On the current tree it returns no hits (exit 1), even though the tag appears in the fixture, both guard tests,
    `stack.sh`, and docs. So there are no false positives.
  - Probe: I added two temporary untracked files, `apps/agent/zz-review-probe.mk` (`go build -tags vrxtestsecrets`)
    and `apps/agent/internal/subsystems/zz-review-probe.txt` (`GOFLAGS=-tags=vrxtestsecrets,netgo`). The line reports
    both. I removed the files afterwards, and the worktree is clean.
  - No other task branch touches `tools/ci.sh`, so the line merges without conflict.

## F9 — D-132 on the screen: fixed

`wireguardStateQuery` (`queries.ts:16-23`) sets `staleTime` = `refetchInterval` = 30 s and `refetchOnWindowFocus:
false`, and the page uses it. A unit test pins all three. Refresh has no cooldown. That was part of F9's suggestion;
I accept it as manual refresh, which D-132 allows.

## UI default (Q2): done

- `newInterfaceDefaults()` gives `{routeAllowedIps: true}`. It is used only for a new interface: an edit keeps the
  stored value.
- The render test checks that the "Route allowed IPs" switch is **checked** in the Add dialog, and that the schema
  default is still `false` (`vpn.ts:434` unchanged).
- The NBMA info line (`nbmaHint`, en + fa, `data-testid="wg-nbma-hint"`) appears for new interfaces. It also points
  full-tunnel peers to their own overlay VRF, which matches F1.

## F3, F10, F11 — docs: fixed

- **F3.** Q1 now says the store is a stand-in behind `vpn.Resolver`, not the channel's receiver, and that a resync
  without material would delete working tunnels. The channel must load material before the first resync (sealed cache
  mandatory), or the descriptors need a keep-existing hook. **Still open for the manager:** add that requirement to
  `docs/decisions/PENDING-secret-channel.md` (not this row's file).
- **F10.** `docs/contracts/proto.md` §11 now gives the event `message` exactly as the code sends it
  (`subsystems/wireguard.go:233`) and tells consumers to read the attributes.
- **F11.** The status file's fix-round table notes that the fake agent applies configurations the real agent refuses
  until the PENDING is answered.

## CI: enough

- **The squash.** `1d4d4159` is a commit object only. Its tree `87f4db7a…` is exactly `f8a61e19^{tree}`, and its parent
  is the merge base `10059d57`. Its subject starts with `contract(schema,proto):`. `173e49b4` adds only 19 lines to
  `docs/status/tasks/F-wireguard.md` on top of `f8a61e19`.
- **The squash run** (`logs/ci/F-wireguard-20260925-095645-675710`):
  - `gitleaks-report.json` = `[]` (1 commit scanned, no leaks);
  - turbo: 30 of 30 tasks succeeded;
  - `apps/agent` built;
  - the summary reports `CI GATE PASSED`.
- **The plain run** (`…-095440-658311`) has exactly one gitleaks finding: `generic-api-key`,
  `apps/api/test/e2e/wireguard.e2e.test.ts:22`, commit `efcf783a`. That is the public schema example key (review §1).
  My rescan agrees: the history has 1 hit and the squash-equivalent diff `10059d57..HEAD` is clean.
- **Why this suffices.** Under D-112 the merger squashes in the worktree and runs `tools/ci.sh --base main` on that
  single commit, which cannot contain `efcf783a`. The ci.sh scan is scoped to `merge-base..tip`, so
  `refs/archive/F-wireguard` is never scanned. The finding is a public value (the D-067 precedent). No history rewrite is
  needed before the merge.

## Re-run by me

```
apps/agent: go test -race -count=1 ./internal/desired/ ./internal/subsystems/ ./internal/agent/ ./internal/descriptors/wireguard/  → ok ×4
apps/agent: go test -count=1 -tags vrxtestsecrets -run TestWireguardFixture ./internal/subsystems/                              → ok
apps/agent: overlay probe TestReviewProbeRouteLoopV6 (6 cases, scratch file)                                                    → PASS
apps/web:   vitest run src/domains/vpn            → 6 tests passed
packages/schema: vitest run src/semantic/wireguard.test.ts → 7 tests passed
git merge-tree --write-tree main(1d3ccf31) task/F-wireguard → clean
```

## Left for the manager (non-blocking)

- Add F3's requirement to PENDING-secret-channel.
- Carry the review's follow-ups F4–F8 and F12–F14, plus the road-warrior no-endpoint warning from F1 above, into
  tech-debt or the named rows.
- Merge notes are unchanged from the review §7. TD-23: turn `extensions` into `RegisterExtension("wireguard", …)`.
  Unions are needed in `fake-agent.ts`, `Connected` and `vpp-code-track.md`. The squash subject starts with
  `contract(schema,proto):`.
