# repair-211-lcp-20261019 WIP

- Branch: `codex/repair-211-lcp-20261019`.
- Base/local SHA before first checkpoint: `cb148aafb`; remote task branch: not yet published.
- Owned files: the three subsystem files and this task's envelope/WIP documents.
- Completed code: exact port of PR 249's three files from `44714075332aa75408924c524ae4a5d0f9e90aa5`; valid binary namespace reads retain existing behavior. Only invalid namespace bytes invoke fixed read-only `show lcp`; root is accepted only after one exact unset status with bounded UTF-8/control-character validation. Transport, malformed, named and ambiguous corroboration remain errors.
- Actual tests: `GOMAXPROCS=2 GOFLAGS=-p=2 tools/ci.sh check --base cb148aafb` passed in 13 seconds, including contract/pattern/secret/trace/slot checks; focused namespace/base-policy tests running; no live integration attempted.
- Remaining work: focused tests/race/vet; first commit/publication; static stripped agent build; unchanged complete quick gate; independent review by manager's reviewer.
- Current failure: none observed in the port; installed source's malformed namespace validation is the reported appliance blocker.
- Exact next command: `GOMAXPROCS=2 go test -p=2 -count=1 ./internal/subsystems -run 'Test(BasePolicy|ProjectBasePolicy)'` from `apps/agent`, alongside repository `tools/ci.sh check --base cb148aafb`.
