# TD-3 WIP — V19 guard (D-095)

Slot 2 (w2). Branch task/TD-3. **Finished — see docs/status/tasks/TD-3.md.**

| part | state |
|---|---|
| (a) create-time sanitizer `internal/vpp/ifsanitize` (+ IPsec SPD, manager add-on), wired into every interface creator, metric, unit + host test | done |
| (b) binding dependencies asserted; classify table Delete refuses while bound (TableUsers + write-only binding records) | done (unit + host) |
| (c) raw deletes in restart simulations / fixtures clear bindings first (`ifsanitize.BeforeDelete`), cleanup order fixed | done |
| (d) `cmd/vrx-vpp-preflight` + `v19_preflight` in ci.sh full (before tests, after rig up) | done (full not run by me) |
