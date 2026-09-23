# RF-3 WIP log

- 2026-09-23 21:17 UTC — started. Read context, envelope, framework (P05a), services schema/proto (P02c).
  Installed: kea 3.0.3, unbound 1.24.2, chrony 4.8 (supports `-p`, `-x`).
  Host note: `chrony.service` is **active + enabled** on the host (host time sync) — not touched by RF-3 (see questions).
  Baseline stat of /etc/kea, /etc/unbound, /etc/chrony saved to /run/vrx-test/w6/etc-stat-before.txt.
- Next: kea renderer (typed structs + encoding/json), unbound, chrony; unit goldens; integration.
- 21:55 UTC — kea renderer + integration green (child kea-dhcp4/6 in ns-w6-a, ctrl-agent 127.0.0.1:3680; config-get drift 0).
  Findings: Kea 3.0 path restrictions (KEA_*_DIR env), `kea -t` checks subnet interface existence (checker runs in the netns),
  Kea re-encodes non-ASCII bytes (descriptions → ASCII escapes), pools reported as CIDR, hex option data without 0x.
- 22:05 UTC — unbound renderer + integration green (127.0.0.1:3653; list_forwards, net.Resolver A + hostile TXT round trip, reload, rollback).
- Next: chrony (server + client chronyd -x), ALLOWLIST, docs, questions, CI.
- 22:20 UTC — chrony renderer + integration green (server + client chronyd -x). Stale-socket fix (unbound/chrony).
  ALLOWLIST section, READMEs, docs/agent/renderers/*.md, questions (Q1–Q7), RF-3.md written. CI GATE PASSED @ 940e51e.
- Closed: all test daemons stopped, ns-w6-a removed, /etc/{kea,unbound,chrony} unchanged. Final CI on the docs commit follows.
