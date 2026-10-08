# PPP carrier package source completion

Branch `codex/pppoe-packaging-20261008`, isolated worktree `ngfw-pppoe-packaging`, base local `e8dbe38a30ad81de783eeb7a70e8ea3127753c32` from manager completion branch. Owner explicitly authorized completion and deferred CI until the final combined tree. This work stages package assets and executes private fixtures only; no host namespace, service, PPP session or package is activated.

## Task envelope and ownership

Owned: `deploy/debian/ngfw/prepare.sh`, `debian/ngfw-agent.install`, `debian/rules`, runtime prerequisites in `debian/control`, `tests/test_prepare.py`, new `tests/test_pppoe_assets.py`, and this receipt. Manager owns integration; `audit_open` owns carrier Go; `audit_open/kernel_helper` owns `scripts/pppoe-kernel-carrier.py` and `scripts/pppoe-carrier-assets/*`. Helper source from local `4e2dc156` is copied read-only into this worktree solely as untracked fixture input because the base predates those files. It is not part of the packaging commit. Integration must include the reviewed helper source and latest receipt corrections.

## Package contract

| Source | Installed location | Mode |
| --- | --- | --- |
| `scripts/pppoe-kernel-carrier.py` | `/usr/lib/ngfw/pppoe-carrier.py` | 0644 |
| `ngfw-pppoe-carrier@.service`, `ngfw-pppoe-broker@.service` | `/usr/lib/systemd/system/` | 0644 |
| `ip-up`, `ip-down`, `ipv6-up`, `ipv6-down` | `/usr/lib/ngfw/pppoe-carrier-hooks/` | 0755 |
| `ngfw-pppoe-carrier.conf` | `/usr/lib/tmpfiles.d/` | 0644 |

The agent package explicitly depends on Python, iproute2, nftables, util-linux, procps, ppp, debianutils and systemd. The helper uses the existing packaged WAN probe and SHA256 sidecar; there is no second unused helper attestation format. `dh_strip` and `dh_dwz` now exclude `ngfw-wan-probe`, matching the existing RA receipt preservation pattern. Build-time `-s -w` occurs before checksum creation. No later Debian transformation may invalidate those bytes.

Fixed service templates remain dormant (`dh_installsystemd --no-enable --no-start`, no `[Install]` sections). Broker requires tmpfiles setup in the host mount namespace; carrier requires VPP and uses its existing private filesystem restrictions. No agent unit, capability, writable path or RA pin changes. Existing identity provisioner is preserved.

## Evidence

- Actual prepare script in a private committed fixture with harmless Go/pnpm boundaries: 3 tests PASS, 0.632s. Confirms fresh build, failure/dirty refusal, complete carrier file contents and modes, install manifest entries, dependencies, WAN/RA checksum bytes, identity asset preservation. Host activation commands are rejecting fixture stubs.
- Dedicated package tests: 3 PASS, 0.067s. Also executed successfully from the prepared output by the prepare fixture. Checks dormant unit dependency graph, exact tmpfiles ownership/modes, executes actual fixed hooks against a mocked dispatcher and verifies argument preservation, and invokes actual Debian strip/dwz rules with observable transformation stubs to prove attested helpers stay unchanged while an unrelated native file is processed.
- `bash -n prepare.sh` and `git diff --check`: pass.
- Existing `test_packaging.py`: 16 tests attempted, 15 pass and one environment error. UID 65534 `os.fchown` fails with EINVAL in this restricted UID namespace. This is not labeled a pass, and its test/assertions are unchanged.

These checks prove source staging and rule contracts, not a built/installed Debian package or a live carrier. No full CI, package installation, live systemd/netns/PPP operation or laboratory traffic test ran. Next: refresh read-only helper inputs at its final checkpoint, rerun the focused fixtures, publish matching tree, and obtain independent review before manager integration and final combined gate.
