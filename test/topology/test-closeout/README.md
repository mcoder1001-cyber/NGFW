# Current-source host closeout

Run existing host tests on fresh private VPP instances without changing the system service. Slots and heavy-command semaphore come from the existing lab tools.

```bash
python3 test/topology/test-closeout/run.py --build --slot 10 --out artifacts/test-closeout/fresh-host-run
```

Commit agent sources first. The runner builds the production agent and race test binary, records source and SHA256 provenance, rejects stale binaries/dirty agent source and nonempty evidence destinations, and records every pass/fail/skip. A skip or absent test result fails acceptance. Each case has an eight-minute Go timeout plus bounded process-group cleanup. The shared VPP PID/NRestarts must remain identical.

Use `--cases tunnels,srv6` for focused reruns. Results prove only the selected executable cases, not composed traffic, screenshots, throughput, DPDK or installed-appliance acceptance. Preserve earlier failing runs.
