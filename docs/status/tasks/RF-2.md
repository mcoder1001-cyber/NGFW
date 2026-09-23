# RF-2 — strongSwan renderer (swanctl/VICI path)

Branch `task/RF-2` (worktree `/root/ngfw-wt/RF-2`, base `main@ff6b91a`, slot 3 / `w3`). Not merged.

## What was built
- `apps/agent/internal/renderers/strongswan` — `renderers.Renderer` for charon:
  - **Render**: `vpn.ipsec.{proposals,tunnels}` (typed `DesiredState` or D-055 `structpb` stand-in) → `strongswan.conf`
    (0640), `swanctl/conf.d/vrx.conf` (0640), `swanctl/conf.d/vrx-secrets.conf` (**0600**, `File.Secret`), from
    `templates/*.tmpl` with strict helpers (`name`, `q`, `tok`, `list`). PSKs resolved from `psk/<name>` refs (D-051) through
    an injected `SecretResolver`, rendered only as `secret = 0s<base64>`. Kernel plugin list is `DaemonConfig.Plugins` (P11 fills).
  - **Validate**: strongSwan has no offline checker → strict settings-grammar parser (`settings.go`) with byte-exact
    round trip + semantic checks (known keys, quoting per key, proposal keyword grammar, masked TS, identity shapes, PSK
    secret/owner consistency, log level ≤ 1, VICI socket). Optional `WithChecker`: `swanctl --load-all` into a **scratch**
    charon (start actions rewritten to none); used by the integration test.
  - **Apply**: atomic write + VICI `load-authority/load-shared/load-pool/load-conn`, unload of everything not rendered
    (`unload-conn` + `terminate`, `unload-shared`, `unload-pool`, `unload-authority`), **convergence check** against
    `list-conns`/`get-shared`/`get-pools`, rollback (restore snapshot + re-load previous files; unload all on a failed first apply).
  - **Retrieve/State**: `version`, `stats`, per-connection `list-conns`/`list-sas` (bounded), `get-shared` (ids only),
    `get-pools`; `*structpb.Struct` (stand-in until P11 adds a state message). `Initiate`/`Terminate`.
  - **Events**: `Watch` subscribes to `ike-updown`, `child-updown`, `ike-rekey`, `child-rekey`; daemon down → 1 Hz `list-sas`
    polling + re-subscribe with backoff 1–30 s; `Event.ToProto`.
  - govici hardening: per-packet 1 MiB bound (`boundedConn`), no unbounded streams (govici drops streamed events silently
    past 128 buffered packets), one session per operation.
- `strongswan/swantest` — reusable harness (P11): netns `ns-w3-{a,b,v}`, veth `w3-a`↔`w3-b` (10.3.250.1/2), charon-systemd per
  instance via the renderers runner, slot lock, SIGTERM to the verified child PID, no pattern kills.
- Docs: `apps/agent/internal/renderers/strongswan/README.md`, `docs/agent/renderers/strongswan.md`; `ALLOWLIST.md` rows
  (swanctl moved Planned → Active; test-only `ip` and `charon-systemd` rows).
- Dependency: `github.com/strongswan/govici v0.8.2` (MIT) — structured VICI instead of parsing `swanctl` text; `golang.org/x/sys`
  moved from indirect to direct (harness `setns`).

## How it was verified

### Unit + integration (`VRX_SLOT=3 VRX_TEST_PREFIX=w3 VRX_INTEGRATION=1 go test -count=1 -v ./internal/renderers/strongswan/...`)
```
--- PASS: TestApplyLoadsConvergesAndIsIdempotent (0.02s)
--- PASS: TestApplyRemovesStaleAndTerminatesSAs (0.01s)
--- PASS: TestApplyFailureRollsBack (0.01s)
--- PASS: TestApplyNotConvergedRollsBack (0.01s)
--- PASS: TestApplyFirstFailureUnloadsEverything (0.01s)
--- PASS: TestApplyDaemonDownRestoresFiles (0.00s)
--- PASS: TestApplyRejectsForeignFiles (0.00s)
--- PASS: TestPlantedSecretNeverLeaks (0.01s)
--- PASS: TestRetrieveState (0.00s)
--- PASS: TestRetrieveBounded (0.01s)
--- PASS: TestInitiateTerminateValidateNames (0.00s)
--- PASS: TestEventsFromVICI (0.00s)
--- PASS: TestWatchReconnectsAndPolls (3.02s)
--- PASS: TestRenderGoldenFull (0.00s)
--- PASS: TestRenderStructInput (0.01s)
--- PASS: TestRenderGoldenModelExtras (0.00s)
--- PASS: TestRenderGoldenEmptyAndProduct (0.00s)
--- PASS: TestRenderDeterministic (0.03s)
--- PASS: TestHostileStringsRejectedOrEscaped (0.20s)
--- PASS: TestRenderRejects (0.00s)
--- PASS: TestDuplicatePSKIdentitiesRejected (0.00s)
--- PASS: TestConnNameMapping (0.00s)
--- PASS: TestIdentityShapes (0.00s)
--- PASS: TestProposalGrammar (0.00s)
--- PASS: TestDaemonConfigChecks (0.00s)
--- PASS: TestParseSettingsRoundTrip (0.00s)
--- PASS: TestParseSettingsRejects (0.00s)
--- PASS: TestRoundTripRequiresCanonicalForm (0.00s)
--- PASS: TestValidateRejectsTamperedFiles (0.01s)
--- PASS: TestStrongswanIntegration (2.51s)
ok  	ngfw/agent/internal/renderers/strongswan	5.944s
```
Covered by unit tests: goldens for every template path (IKEv1/IKEv2, psk/pubkey, tunnel/transport, esp/ah, multiple
children, pools, authorities, dpd incl. IKEv1 timeout, mobike, encap, reauth, rekey bytes/packets, anti-replay off, ESN,
route-based if_id, product vs test strongswan.conf); hostile strings (`"; rm -rf /`, `}\ninclude /etc/passwd\n{`, `#`,
unicode, 5 KB, CR/LF/NUL/ESC/U+2028, invalid UTF-8, `${…}`, `a:b`, `a.b.c`, quotes, backslash, braces) in every user field
incl. the tunnel name and the PSK value → rejected (`ErrInput`/`ErrUnsafe`) or confined to a quoted value/comment; the
`include` injection is rejected by name validation, the parser and control-char rejection; tampered files rejected by
Validate; fake-charon Apply/idempotency/stale unload+terminate/rollback/not-converged/first-apply/daemon-down; planted secret.

### Integration evidence (two stock charon-systemd 6.0.4 in ns-w3-a / ns-w3-b, kernel-netlink, no VPP)
```
=== RUN   TestStrongswanIntegration
strongswan_integration_test.go:118: strongSwan binaries: root /run/vrx-test/w3/swan-stock/root (charon-systemd, swanctl)
strongswan_integration_test.go:177: rendered endpoints are in 10.3.0.0/16; IKE binds only inside ns-w3-a / ns-w3-b
strongswan_integration_test.go:184: charon a started in ns-w3-a, VICI /run/vrx-test/w3/swan/a/charon.vici
strongswan_integration_test.go:184: charon b started in ns-w3-b, VICI /run/vrx-test/w3/swan/b/charon.vici
strongswan_integration_test.go:184: charon v started in ns-w3-v, VICI /run/vrx-test/w3/swan/v/charon.vici
strongswan_integration_test.go:186: test daemons (argv0 under /run/vrx-test/w3/swan): pids [1813971 1813997 1814019]
strongswan_integration_test.go:204: Validate a+b: strict parser ok; swanctl --load-all into scratch charon v ok (v has w3-ab, start_action none, 0 SAs)
strongswan_integration_test.go:247: Apply a+b ok: list-conns w3-ab IKEv2 local [10.3.250.1] remote [10.3.250.2] children [{w3-ab TUNNEL 3600 [10.3.1.0/24] [10.3.2.0/24]}]; get-shared [ike-w3-ab]; secrets file mode -rw-------
strongswan_integration_test.go:271: list-sas (a, via Retrieve; PSK never present):
    {
      "a": {
        "name": "w3-ab",
        "tunnel": "w3-ab",
        "uniqueId": "1",
        "version": "2",
        "state": "ESTABLISHED",
        "localHost": "10.3.250.1",
        "localPort": "4500",
        "localId": "10.3.250.1",
        "remoteHost": "10.3.250.2",
        "remotePort": "4500",
        "remoteId": "10.3.250.2",
        "initiator": true,
        "natAny": false,
        "encrAlg": "AES_GCM_16",
        "encrKeysize": "256",
        "prfAlg": "PRF_HMAC_SHA2_256",
        "dhGroup": "CURVE_25519",
        "establishedSec": 0,
        "rekeySec": 14398,
        "reauthSec": 0,
        "children": [
          {
            "name": "w3-ab",
            "uniqueId": "1",
            "reqId": "1",
            "state": "INSTALLED",
            "mode": "TUNNEL",
            "protocol": "ESP",
            "encap": false,
            "spiIn": "ca7f76b6",
            "spiOut": "cadbe3a1",
            "encrAlg": "AES_GCM_16",
            "encrKeysize": "256",
            "esn": false,
            "bytesIn": 0,
            "packetsIn": 0,
            "bytesOut": 0,
            "packetsOut": 0,
            "rekeySec": 3388,
            "lifeSec": 3960,
            "installSec": 0,
            "localTs": [
              "10.3.1.0/24"
            ],
            "remoteTs": [
              "10.3.2.0/24"
            ]
          }
        ]
      },
      "b.remoteHost": "10.3.250.1",
      "b.state": "ESTABLISHED"
    }
strongswan_integration_test.go:276: ip -n ns-w3-a xfrm state (keys removed):
    src 10.3.250.1 dst 10.3.250.2
    	proto esp spi 0xcadbe3a1 reqid 1 mode tunnel
    src 10.3.250.2 dst 10.3.250.1
    	proto esp spi 0xca7f76b6 reqid 1 mode tunnel
strongswan_integration_test.go:283: events: ike-updown up + child-updown up received from VICI
strongswan_integration_test.go:293: idempotent re-Apply: IKE_SA #1 still ESTABLISHED
strongswan_integration_test.go:315: failed Apply rolled back: error "load connection w3-bad: strongswan: daemon error: vici load-conn: vici: command failed: invalid value for: certs, config discarded"; vrx.conf restored byte-for-byte; charon still has only w3-ab with IKE_SA #1
strongswan_integration_test.go:337: charon a restarted (SIGTERM to the PID the harness spawned, then started again): re-Apply loaded w3-ab from the files, IKE_SA #1 ESTABLISHED
strongswan_integration_test.go:344: events: Watch reported daemon down and, after re-subscribing, daemon up
strongswan_integration_test.go:388: after terminate + unload: Retrieve a = conns [] sas [] sharedSecrets []; xfrm state/policy empty in both namespaces
strongswan_integration_test.go:426: planted secret: not found in 14 sources (vrx.conf, strongswan.conf, charon logs a/b, renderer log, Retrieve, 8 events, errors)
strongswan_integration_test.go:116: cleanup: daemons stopped (none left under /run/vrx-test/w3/swan), namespaces deleted; /etc unchanged: /etc/strongswan.conf: absent; /etc/swanctl: absent; /etc/strongswan.d: absent; /usr/lib/ipsec: absent; 
--- PASS: TestStrongswanIntegration (2.51s)
```
Test log: `grep -c VRX_TEST_PSK /root/ngfw-wt/logs/RF-2-integ-final.log` → `0` (the PSK exists only base64-encoded in the 0600
secrets file and in charon). Log files: `/root/ngfw-wt/logs/RF-2-integ-final.log`, `/root/ngfw-wt/logs/RF-2-ci-1.log`.

### Acceptance checks (after the runs)
```
$ grep -rn "sh -c\|bash -c" internal/renderers/strongswan        → (no output, exit 1)
$ grep -rn "exec.Command" internal/renderers/strongswan           → (no output, exit 1)
$ pgrep -af /run/vrx-test/w3/swan                                 → only the grep's own shell; no test daemon
$ systemctl is-active strongswan-starter strongswan               → inactive / inactive  (is-enabled: not-found)
$ stat /etc/swanctl /etc/strongswan.conf                          → No such file or directory (before and after; test asserts unchanged)
$ ip netns list | grep -c w3                                      → 0
$ ls /run/vrx-test/w3                                             → swan-stock  swan.lock
$ golangci-lint run ./internal/renderers/...                      → 0 issues
```

### CI gate (`tools/ci.sh --base main`)
```

== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m01s
  install (pnpm --frozen-lockfile --prefer-offline)   0m06s
  generate + generated-output gate                   0m22s
  forbidden patterns (+ gitleaks)                    0m04s
  lint · typecheck · unit tests · build (turbo)   0m20s
  apps/agent: make lint test build                   0m43s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  mode quick · wall time 1m39s · logs /root/ngfw-wt/logs/ci/RF-2-20260924-020548-1790303

CI GATE PASSED
CI GATE PASSED
```
(gitleaks: "no leaks found"; contract guard: "no contract files changed".)

## Environment finding (important)
strongSwan is **not installed** on this host (the prompt assumed it was). I did not install it (creates `/etc/swanctl`,
`/etc/strongswan.conf`, enables `strongswan.service`). The stock Ubuntu 6.0.4 debs were extracted with `apt-get download` +
`dpkg -x` into `/run/vrx-test/w3/swan-stock/root` (tmpfs; lost on reboot; README has the 3 commands). The harness runs daemons
and swanctl on a locked OS thread that enters a private mount namespace (propagation private) with read-only overlays of the
extracted `usr/lib` and `usr/sbin` (stock binaries hard-code `/usr/lib/ipsec/plugins`; `/run` is noexec) and then the rig netns.
The test asserts the overlay never appears in the host namespace. If strongSwan gets installed, the harness uses it directly.

## Out of scope / left undone
- Wiring into the commit engine / agent main (like RF-1: P05/P11). No `DryRun` method (not in the interface).
- Certificates/PKI: `pubkey` connections and `authorities` render and load (cert files read from `x509*/`), untested end to end
  (F-pki-basic); private keys not loaded. EAP/remote access: `pools` render+load from the model only; no document mapping.
- `natT=false` is not representable (charon always detects NAT); `vrf`/`underlayVrf`, `vpn.ipsec.settings` not rendered (P11/DF-5).
- IKEv1 rendered, not tested live. No packet test (not requested).
- A SIGKILLed charon leaves xfrm states (kernel-netlink); the kernel-vpp equivalent is P11/DF-5's to reconcile.

## Open questions → `docs/status/tasks/RF-2-questions.md`

## Decisions (for the LOG)
| decision | options considered | why |
|---|---|---|
| Test daemons from extracted stock debs + private mount-ns overlay | (a) install strongswan (b) extract + overlay (c) skip integration | (a) touches /etc + enables the unit, forbidden; (c) no evidence |
| charon-systemd instead of `/usr/lib/ipsec/charon` in tests | (a) charon + `unshare -m` tmpfs /var/run (b) charon-systemd | no pid-file setting in strongswan.conf(5); charon-systemd writes none; product (P11) runs charon-systemd too |
| Tunnel name `.` → `+` in section names | (a) reject names with `.` (b) bijective map | `.` is the settings path separator; `+` is outside objectName so the map reverses |
| PSK rendered as `0s<base64>` | (a) quoted string (b) base64 | any bytes, no escaping; plaintext never in any file |
| Explicit remote `id` = remoteAddr when unset (IP) | (a) leave %any (b) explicit | charon must not accept an arbitrary identity for a PSK |
| AEAD IKE proposal without prf → `prfsha256` | (a) reject (b) default | schema allows prf unset; charon needs a PRF with AEAD |
| Renderer owns charon's VICI config (unloads unrendered conns/secrets/pools) | (a) own all (b) track own names | same semantics as `swanctl --load-all`; stateless across agent restarts |
| PSK ≥ 8 bytes | (a) any (b) ≥ 8 | whole site-to-site auth rests on it |
| Secrets golden stored as `secret = <redacted>` | (a) base64 fixture (b) redacted + decoded assert | gitleaks flags base64 fixture PSKs |
| govici per-connection bounded streaming + 1 MiB packet cap | (a) trust govici (b) harden | govici drops events silently past 128 packets and allocates announced lengths |


---

## Review fixes (review `709e08e`, APPROVE WITH CHANGES)

`git merge main` done first (`3c6b214`): the ALLOWLIST.md conflict was two "Planned" blocks that each side had
moved to *Active* (RF-3: kea/unbound/chrony; RF-2: swanctl) — resolved by dropping both Planned blocks; every row
exists in the Active sections. Fix round commit: `454194b`.

| finding | fix |
|---|---|
| **H1** SAs keep old TS/PSK/proposals | Apply now converges the **live SAs**: per-connection fingerprints (IKE: version, addrs, proposals, auth, ids, PSK+owners; child: TS, ESP/AH proposals, mode, if_ids, replay window) diffed against the previous files → affected IKE_SAs/CHILD_SAs terminated by unique id (graceful DELETE, forced after 3 s); every live SA that visibly contradicts the loaded config (TS outside configured prefixes, id/version mismatch, child gone) is terminated as well; success only when `list-sas` shows none of them. `State.staleSas`. Rollback only re-evaluates SAs against the failed plan if its reconciliation ran |
| **M1** flap on soft edits, duplicate CHILD_SA | `start_action = start` is withheld from the VICI load (kept in the files for the boot path): charon's replace path can no longer undo/redo it. The renderer initiates a start child only when it has no CHILD_SA and no IKE_SA is connecting. Soft edits leave the SA alone. `Renderer.Impact(files)` → `reestablish`/`update`/`add`/`remove` per connection for the commit engine/UI; documented in README + `docs/agent/renderers/strongswan.md` |
| **M2** implicit product paths, own-all | `Paths` is required (`New()` without `WithPaths` refuses to render/apply). `WithOwnerPrefix(p)`: rendered names must carry `p`; unload/converge/State touch only owned conns/pools/authorities and `ike-<p>…` secrets. Integration uses `WithOwnerPrefix("w3")`; shared-host rule proposed in questions Q9 |
| **M3** charon restart invisible | `State.daemonStartedAt` (`stats.uptime.since`) vs `ackedStartedAt` persisted in `Paths.BootRecord` → `restarted` (until `AckRestart`); `Watch` sends `daemon/restarted` on (re-)subscription. Harness: `Crash` (SIGKILL) + `FlushXfrm`; `Stop` flushes after a SIGKILL fallback. P11/DF-5 reconcile step in questions Q10 |
| **M4** silent event loss | `Watch` re-lists SAs every 30 s while subscribed and immediately when its buffer is full (govici drops then): `resync` event + `poll` diffs |
| **L1** names 61–63 fail late | tunnel names capped at 60 (`MaxConnNameLen`), validated before render with a clear error; schema `max(60)` proposed (questions Q7) |
| **L2** global text redaction | removed: Retrieve/State/events are not rewritten (no key material by construction); only `secret = …` in errors/tool output is masked. Test: a PSK equal to `ESTABLISHED` leaves state intact |
| L3 bounded listings fail | `State.truncated` instead of an error; `unloadStale` works on a partial listing |
| L4 checker staging | documented; integration test sets `TMPDIR` to the slot's tmpfs |
| L5 strongswan.conf never applied | Apply returns `*ActionRequired{restart}` (D-079 shape, re-derived each Apply) when charon's loaded plugins lack a rendered one |
| L6 harness orphans on `go test -timeout` | documented manual cleanup in README (the runner has no `Pdeathsig` hook — would need a `helpers_exec.go` change) |
| I1 | AEAD default PRF noted for P11's schema help text (unchanged code) |

### Unit tests for the fixes (`go test -race ./internal/renderers/...` all ok; `golangci-lint` 0 issues)
```
--- PASS: TestRetrieveBounded (0.01s)
--- PASS: TestRedactionIsFieldBased (0.00s)
--- PASS: TestApplyReestablishesChangedSAs (0.01s)
--- PASS: TestApplyTerminatesContradictingSAWithoutHistory (0.00s)
--- PASS: TestApplyFailsWhenStaleSAStays (0.01s)
--- PASS: TestApplySoftChangesKeepSAs (0.02s)
--- PASS: TestOwnerPrefix (0.01s)
--- PASS: TestRestartDetection (0.00s)
--- PASS: TestWatchResync (0.02s)
--- PASS: TestTunnelNameCap (0.00s)
```

### Integration (`VRX_SLOT=3 VRX_TEST_PREFIX=w3 VRX_INTEGRATION=1 go test -count=1 -v ./internal/renderers/strongswan/...` → 39 PASS, 0 FAIL; list-sas JSON body elided)
```
=== RUN   TestStrongswanIntegration
strongswan_integration_test.go:131: strongSwan binaries: root /run/vrx-test/w3/swan-stock/root (charon-systemd, swanctl)
strongswan_integration_test.go:195: rendered endpoints are in 10.3.0.0/16; IKE binds only inside ns-w3-a / ns-w3-b
strongswan_integration_test.go:202: charon b started in ns-w3-b, VICI /run/vrx-test/w3/swan/b/charon.vici
strongswan_integration_test.go:202: charon v started in ns-w3-v, VICI /run/vrx-test/w3/swan/v/charon.vici
strongswan_integration_test.go:202: charon a started in ns-w3-a, VICI /run/vrx-test/w3/swan/a/charon.vici
strongswan_integration_test.go:204: test daemons (argv0 under /run/vrx-test/w3/swan): pids [2055574 2055598 2055621]
strongswan_integration_test.go:222: Validate a+b: strict parser ok; swanctl --load-all into scratch charon v ok (v has w3-ab, start_action none, 0 SAs)
strongswan_integration_test.go:265: Apply a+b ok: list-conns w3-ab IKEv2 local [10.3.250.1] remote [10.3.250.2] children [{w3-ab TUNNEL 3600 [10.3.1.0/24] [10.3.2.0/24]}]; get-shared [ike-w3-ab]; secrets file mode -rw-------
strongswan_integration_test.go:289: list-sas (a, via Retrieve; PSK never present):
strongswan_integration_test.go:294: ip -n ns-w3-a xfrm state (keys removed):
    src 10.3.250.1 dst 10.3.250.2
    	proto esp spi 0xc8835600 reqid 1 mode tunnel
    src 10.3.250.2 dst 10.3.250.1
    	proto esp spi 0xc20acb95 reqid 1 mode tunnel
strongswan_integration_test.go:301: events: ike-updown up + child-updown up received from VICI
strongswan_integration_test.go:311: idempotent re-Apply: IKE_SA #1 still ESTABLISHED
strongswan_integration_test.go:333: failed Apply rolled back: error "load connection w3-bad: strongswan: daemon error: vici load-conn: vici: command failed: invalid value for: certs, config discarded"; vrx.conf restored byte-for-byte; charon still has only w3-ab with IKE_SA #1
strongswan_integration_test.go:378: M1: Impact map[w3-ab:update]; Apply(dpd 30→20, start_action none→start): IKE_SA #1 kept, 1 CHILD_SA (no duplicate)
strongswan_integration_test.go:403: H1 probe 1: Apply(b, remote_ts 10.3.1.0/25) returned after terminating the /24 CHILD_SA; b: stale 0, xfrm policy has no 10.3.1.0/24
strongswan_integration_test.go:419: H1 probe 1: a followed; IKE_SA #2 CHILD_SA #2 INSTALLED local_ts [10.3.1.0/25] remote_ts [10.3.2.0/24] (b: remote [10.3.1.0/25])
strongswan_integration_test.go:445: H1 probe 2: PSK rotated on b then a: old IKE_SA #2 gone, IKE_SA #3 ESTABLISHED (authenticated with the new key)
strongswan_integration_test.go:468: M3: after SIGKILL + restart: State.restarted=true (started "Sep 24 02:32:50 2026", acked "Sep 24 02:32:49 2026"), Watch sent daemon/restarted; xfrm residue in ns-w3-a:
    src 10.3.250.1 dst 10.3.250.2
    	proto esp spi 0xca360273 reqid 1 mode tunnel
    src 10.3.250.2 dst 10.3.250.1
    	proto esp spi 0xc9334035 reqid 1 mode tunnel
strongswan_integration_test.go:484: M3: residue flushed, AckRestart → restarted=false; re-Apply loaded w3-ab and initiated it (start action): IKE_SA #1 ESTABLISHED
strongswan_integration_test.go:491: events: Watch reported daemon down and, after re-subscribing, daemon restarted
strongswan_integration_test.go:541: after terminate + unload: Retrieve a = conns [] sas [] sharedSecrets []; xfrm state/policy empty in both namespaces
strongswan_integration_test.go:581: planted secret: not found in 15 sources (vrx.conf, strongswan.conf, charon logs a/b, renderer log, Retrieve, 16 events, errors)
strongswan_integration_test.go:129: cleanup: daemons stopped (none left under /run/vrx-test/w3/swan), namespaces deleted; /etc unchanged: /etc/strongswan.conf: absent; /etc/swanctl: absent; /etc/strongswan.d: absent; /usr/lib/ipsec: absent; 
--- PASS: TestStrongswanIntegration (2.86s)
```
`grep -c VRX_TEST_PSK /root/ngfw-wt/logs/RF-2-fix-integ-final.log` → `0`. Log: `/root/ngfw-wt/logs/RF-2-fix-integ-final.log`.

### After the runs
```
pgrep -af /run/vrx-test/w3/swan          → none (only the grep's shell)
ip netns list | grep -c w3               → 0
ip xfrm state | wc -l   (host)           → 0
findmnt | grep -c swan-stock             → 0
ls /run/vrx-test/w3                      → swan-stock  swan.lock
systemctl is-active strongswan strongswan-starter → inactive inactive
/etc/strongswan.conf, /etc/swanctl       → absent before and after (asserted by the test)
```

### CI gate (`tools/ci.sh --base main`)
```
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m01s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   0m37s
  forbidden patterns (+ gitleaks)                    0m03s
  lint · typecheck · unit tests · build (turbo)   0m28s
  apps/agent: make lint test build                   0m48s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  warnings:
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(RF-2): findings
  mode quick · wall time 2m02s · logs /root/ngfw-wt/logs/ci/RF-2-20260924-023251-2055836

CI GATE PASSED
```
gitleaks: "no leaks found"; contract guard: "no contract files changed in the 11 commit(s)". The first fix-round CI run
failed gitleaks on a test literal `secret = 0s<base64 of "ABCDEFGH">` (generic-api-key) in my own, unreviewed fix-round
commits `49f20a4`/`3534a7a`; per the CI instruction ("recreate the commits without it") they were squashed into
`454194b` with the literal replaced by the allow-listed `VRX_TEST_PSK_RF2_masked` form. Nothing reviewed or merged was
rewritten.

### Decisions (fix round)
| decision | options considered | why |
|---|---|---|
| Renderer drives `start` actions; charon keeps `trap` | (a) load start_action as rendered (b) withhold start, initiate when no CHILD_SA | (a) is exactly review M1 (charon undoes/re-runs start on every replace) |
| SA convergence = fingerprint diff + visible-contradiction check | (a) terminate on any change (b) security fingerprint only (c) fingerprint + live check | (c) also heals SAs from unknown history (agent restart, missing files) without flapping soft edits |
| Tunnel name cap 60 (manager) instead of raising the section-name limit to 68 (reviewer) | (a) nameRe 68 (b) cap 60 + schema max(60) proposal | manager's instruction; keeps every derived name ≤ 64 |
| Restart detection by `stats.uptime.since` + ack file | (a) VICI pid (not exposed) (b) since + persisted ack | VICI has no pid; persisted ack survives agent restarts |
