# PR #244 — independent review (2026-10-20, reviewer agent, read-only)

Head `821b3888d`. **Verdict: APPROVE WITH CHANGES** (one MAJOR, docs only; no defect in the code change).

Checks: `bash -n` 0; `shellcheck --external-sources` 0; fixture suite in a scratch worktree `Ran 33 tests in 668.215s — OK`.

Questions: (1) the no-download/no-dependency-change guarantee holds and is stronger than `--no-download` (APT sees only dpkg status and the four staged archives; `--allow-downgrades` can affect only the ngfw packages; removals blocked by `--no-remove` and the plan guard; an unmet dependency fails in `--simulate` under `set -e`). (2) the unhidden `/var/lib/apt/lists` does not matter (lists are opened only for configured sources; the cached index is rebuilt; `/dev/null` for SourceParts is APT's own idiom); the host's `apt.conf.d` still applies as before, backed by the plan guard. (3) the receipt is honest apart from findings 1, 3, 4.

Findings:
1. MAJOR — attribution: `vpp.service` requires `ngfw-firstboot.service` on every appliance, because ngfw-meta ships the drop-in `deploy/debian/ngfw/assets/vpp-firstboot.conf` (`ngfw-meta.install:4`, `test_packaging.py:288`), not "on this host"; with `dh_installsystemd --no-enable --no-start` the stop happens on every upgrade, so every run of the tool restarts VPP, contradicting `ngfw-meta.postinst:4` and the operator doc's "makes no VPP restart request". Fix: correct the receipt/DEFERRED attribution, add a sentence to deploy-hosts.md (ngfw-meta upgrade stops and restarts VPP, 6–9 s; if dpkg fails after the stop, VPP stays down until manual recovery).
2. MINOR — no fixture pins the fix (deleting both `-o` options keeps 33/33 green). Fix: assert both options twice in the install log and no `--no-download`.
3. MINOR — receipt says NGFW_VPP_IN_TESTS "only skips tests/run.sh"; `verify.sh:98` also skips shellcheck of the four scripts.
4. MINOR — DEFERRED-ACCEPTANCE "recovery from a failed install step" overclaims (failure before dpkg changed nothing; recovery after a partial dpkg/prerm failure is NOT RUN); the host-211 run is missing.
5. MINOR (R8) — `deploy/debian/bundle/install.py:173` still passes `--no-download` (asserted by `test_install.py:141`); probably broken on apt 3 too. Out of scope; follow-up.
6. NIT — hidden sources let root rewrite `/var/cache/apt/*pkgcache.bin`; harmless (rebuilt next run); install.py avoids it with `Dir::Cache::pkgcache=` / `srcpkgcache=`.
