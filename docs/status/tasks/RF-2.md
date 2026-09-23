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
