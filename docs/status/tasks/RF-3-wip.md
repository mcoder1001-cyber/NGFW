# RF-3 WIP log

- 2026-09-23 21:17 UTC — started. Read context, envelope, framework (P05a), services schema/proto (P02c).
  Installed: kea 3.0.3, unbound 1.24.2, chrony 4.8 (supports `-p`, `-x`).
  Host note: `chrony.service` is **active + enabled** on the host (host time sync) — not touched by RF-3 (see questions).
  Baseline stat of /etc/kea, /etc/unbound, /etc/chrony saved to /run/vrx-test/w6/etc-stat-before.txt.
- Next: kea renderer (typed structs + encoding/json), unbound, chrony; unit goldens; integration.
