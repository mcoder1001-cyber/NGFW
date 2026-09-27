# F-nat46 — questions for the manager

- **Q1 (stateful NAT46)**: VPP 26.06 has no stateful IPv4→IPv6 translator. Options: explicit out-of-scope decision (D-entry)
  or a `docs/vpp-code-track.md` item (2x rule). Not decided here.
- **Q2 (wiring)**: files_owned covers only the library. Wiring needs a contract (`nat.nat46{clientPrefix, interfaces[],
  mappings[]{name, ipv4, ipv6, mtu?}}`, additive), a `desired/nat46.go` builder feeding mapnat's `map.domain`/`map.interface`,
  and mapnat being wired (pending on F-det44-map-dslite-cnat). Follow-up row, or extend files_owned?
- **Q3 (ownership with F-det44-map-dslite-cnat)**: NAT46 domains are `map.domain/nat46-<name>`; the nat.map builder must reject
  domain names starting `nat46-`, and MAP-T interface keys are shared (one `map.interface/<if>/map-t` for both). Agree.
- **Q4 (trailer)**: common.md asks for "Claude Fable 5.1" in the trailer; the session harness mandates "Claude Opus 5.5".
  Commits use the harness trailer.
