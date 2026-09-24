# TD-3 WIP — V19 guard (D-095)

Slot 2 (w2). Branch task/TD-3. **Fix round 2 in progress (started 08:00).**

| part | state |
|---|---|
| merge main (DF-5, F-startup-apply, V24) | done 6e42c16 (conflict in docs/vpp-code-track.md: kept V23 + V24) |
| H1 ipsec.itf + wireguard.interface through Acquire/BeforeDelete | done + unit tests; guard test `TestEveryInterfaceCreatorIsSanitized` (fails on main's code: itf.go:67, interface.go:89) |
| M1 re-read on stolen holes, cap 16, fail closed (ErrCapped), capped metric | done + unit tests |
| M2 holder instance 16383↓16000, docs, schema question | done + unit tests; CONTRACT question in TD-3-questions.md |
| L5 ci.sh after-failed-tests pre-flight, all FAIL lines | done; harness-tested |
| host runs (ifsanitize, ipsec, wireguard, slot 2) | done 13:18–13:27, all PASS, NRestarts 0→0 |
| status "Fix round 2" section | done; CI next |
