# RF-1 — WIP (FRR renderer framework)

Updated: 2026-09-24 00:30 (slot 12, worker on host)

## Done
- Read context, envelope, P12, D-049/D-055/D-056, renderer contract.
- Host probe (FRR 10.7.1): findings that shape the design
  - `vtysh -N <ns>` appends `/<ns>` to both `--config_dir` and `--vty_socket` → layout `<ConfDir>/<ns>/frr.conf`, sockets `<RunDir>/<ns>/`.
  - mgmtd has no `-f` and binds `mgmtd_fe.sock`/`mgmtd_be.sock` in `/var/run/frr/<ns>` regardless of `--vty_socket` →
    test harness symlinks `/run/frr/w12 → /run/vrx-test/w12/frr/run/w12` (removed in cleanup).
  - daemons refuse `-u root` (vty group) → run as frr, test run dir chowned frr:frr.
  - running-config canonical static form: `ip route P NH tag T DIST`; vrf statics inside `vrf X … exit-vrf`.
  - FRR 10.7 daemons ignore `hostname` (always the system hostname) → rendered only when set.
  - empty `line vty` never appears in running-config → perpetual frr-reload delta → not rendered when empty.
  - `show vrf json` does not exist in 10.7 (text `show vrf` only).
  - frr-reload default `--logfile /var/log/frr/frr-reload.log` → always pass `--logfile`.

## Next
- package `internal/renderers/frr` (paths, model, sections, templates, renderer, state, events) + `frr/frrtest` harness
- golden + hostile tests, integration test, docs, CI
