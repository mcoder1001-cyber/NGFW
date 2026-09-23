# RF-2 — WIP (strongSwan renderer, swanctl/VICI path)

Updated: 2026-09-24 (start)

## State
- [x] Read context, ops, task, template, envelope, P11, host rules, LOG, RF-1 review, vpn schema/proto
- [x] Finding: strongSwan is **not installed** on this host (no /usr/lib/ipsec, no /usr/sbin/swanctl) although the task prompt says so.
      Not installing it (would create /etc/swanctl, /etc/strongswan.conf and enable strongswan.service — all off-limits).
      Instead: stock Ubuntu 6.0.4 debs extracted with `apt-get download` + `dpkg -x` into `/run/vrx-test/w3/swan-stock/root`;
      the harness runs them in a private mount namespace with a read-only overlay of the extracted `usr/lib` + `usr/sbin`
      (stock binaries hard-code `/usr/lib/ipsec/plugins`; `/run` is noexec). Prototype verified: charon-systemd 6.0.4 + VICI answer.
- [x] govici v0.8.2 (MIT) added to apps/agent/go.mod
- [ ] model + escaping + templates + golden tests
- [ ] strict settings parser (Validate round-trip) + VICI message builder
- [ ] Apply (VICI load/unload, convergence, rollback), Retrieve, events
- [ ] swantest harness + integration (two charons, IKEv2 PSK SA)
- [ ] docs, ALLOWLIST, CI gate, RF-2.md

## Decisions so far (to be copied to LOG)
- charon-systemd (no pid file) instead of /usr/lib/ipsec/charon for the test daemons: `man strongswan.conf` has no pid-file setting and
  `/var/run/charon.pid` is compile-time; charon-systemd writes none, so no private /var/run is needed.
- Connection names: `.` is a settings path separator (NAME excludes `. , : { } = " #`), so tunnel names (objectName allows `.`) are mapped
  `.` → `+` (bijective: `+` is not in objectName's charset).
- PSKs are rendered as `secret = 0s<base64>` (swanctl's base64 form): any byte string is representable and no escaping is involved.
