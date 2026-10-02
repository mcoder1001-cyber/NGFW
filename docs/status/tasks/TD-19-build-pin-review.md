# TD-19 build pins / provision fixture independent R1/R2/R5 review

Frozen head `f7d4f694fb3d105c68ab5b3795646e6c65982d97`, delta from `cb6fdee8`, isolated task/TD19-build-pin-review. No product authorship or real apt/network/install/host mutation.

## MAJOR — newly installed pinned Go may not be used

`scripts/20-install-build.sh:51–54`: PATH appends `/usr/local/go/bin`, and persisted profile fragment also appends it. A preexisting earlier binary such as `/usr/bin/go` with old version triggers installation of the verified1.26.0 archive, but remains first on PATH. All three subsequent `go install` commands therefore use the old binary; the pin is not actually enforced for bootstrap consumers. This existed in old installation structure but is material to this delta's exact pinned toolchain claim.

Fix: consistently select/prepend the verified Go binary for the current invocation and persisted future profile, verify exact selected version after installation, and add an offline earlier-PATH-old-Go regression covering actual installer selection without host mutations. Do not merely check a downloaded archive's SHA or print configuration pins. If accepting an already exact-version external binary, make the selection policy explicit so a nonexistent `/usr/local/go/bin` is not assumed.

Personally reproduced shell selection with temporary earlier/verified directories and executable fake Go scripts, executing the script's same `export PATH=$PATH:<verified-bin>; go version` semantics:

```
Same append-PATH selection with earlier old binary: OLD exit0
```

## Other checks

- Fixed Go1.26.0 digest exactly matches supplied reviewed official checksum `aac1b08a0fb0c4e0a7c1555beb7b59180b05dfc5a3d62e40e9de90cd42f88235`. Unset-only parameter expansion rejects explicitly empty/different/malformed checksum overrides before APT. Download uses private unique mktemp path with cleanup; sha256sum check precedes destructive `/usr/local/go` replacement. No checksum override bypass, fixed user-controlled URL, shell injection or secret introduced.
- Generator versions exactly pinned to protobufv1.36.12, grpcv1.6.2, govppv0.13.0 and match reviewed repo CI/module assertions. `@latest` removed for these tools. Existing npm fallback and preexisting build dependency policy are outside this narrow delta, not newly approved by this report.
- Provision fixture extracts and executes actual cmd_provision source with all remote operations recorded/refused; proves artifact preflight and approval gate before remote changes, and remote OS refusal before staging. It does not simulate successful real provisioning or prove runtime installation acceptance.
- Bounds are constant-sized script/config/test inputs; no hot-path/unbounded loop or new background process introduced. Restriction to x86_64 is explicit; check-config is pure and does not execute installations.

## Personally executed

```
python3 docs/status/tasks/TD-19-test-build-preflight.py
Ran3 tests in0.020s — OK
python3 docs/status/tasks/TD-19-test-provision-order.py
Ran3 tests in0.043s — OK
bash -n scripts/20-install-build.sh tools/lab
exit0
```

No real archive download, APT, remote apply, Go installation or appliance boot executed. Full TD19 remains unfinished; P10 dependency/limited source exception remain unchanged.

**Verdict: BLOCK (1 MAJOR, pinned toolchain selection). Fix and narrow independent recheck required before merging this delta.**
