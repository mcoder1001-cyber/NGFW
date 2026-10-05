# HA full independent R2 security review
Frozen source: c42307e12d124aea01640b60a454c652270154ae. Full new HA security scope reviewed against merge base 7b507db5a0e750146c94b8cc29c681ea2c792a74; no inherited R2 approval assumed.

BLOCKER — test/topology/ha-state-sync/driver.py:12–17: bearer credential forwarded across redirects. fetch uses urllib.request.urlopen with Authorization; default HTTPRedirectHandler copies that header to a different HTTPS origin (and permits HTTP downgrade). A compromised appliance or its trusted proxy can return Location pointing to an attacker, disclosing an administrator token even in default observation mode. README explicitly retains this driver, so fixing acceptance.py alone leaves a callable credential-leaking path. Fix: use the same credential-free HTTPS origin validation and redirect-refusing opener as acceptance.Api, with a regression covering foreign-origin and downgrade redirects; do not print credentials. Confirmed synthetic reproduction, no network:
```python
import urllib.request
r = urllib.request.Request('https://appliance.invalid/api/v1/state/ha/sync', headers={'Authorization': 'Bearer NGFW_TEST_PSK_R2'})
s = urllib.request.HTTPRedirectHandler().redirect_request(r, None, 302, 'redirect', {}, 'https://foreign.invalid/capture')
assert s.host == 'foreign.invalid' and s.get_header('Authorization') == r.get_header('Authorization')
print('REPRODUCED: default urllib redirect forwards Authorization to foreign HTTPS origin (synthetic, no network)')
```
Executed with PYTHONDONTWRITEBYTECODE=1 python3 stdin in reviewer worktree; output exactly the REPRODUCED line.

Other reviewed boundaries: REST inherits global AuthGuard; resync explicitly requires admin and records key-free mutation audit. Agent actions require startup globals-owner role, enabled EI running state, default VRF, and exact live endpoint agreement. Slots require existing singleton values; only globals owner sets/reset endpoints. Correlated completion ignores foreign PID events, rejects missed/timeout results, and waits under a bounded context. Endpoint address/port/MTU/refresh inputs are bounded; no config input reaches shell. RPC transport remains the existing local privileged socket boundary.
Native HA UDP is unauthenticated, as documented in projection and user docs. Dedicated trusted sync network and existing reachability policy are mandatory deployment inputs; cluster secretRef only authenticates configuration sync. Address/interface matching does not authenticate packets or enforce network isolation. This review does not certify hostile-network safety.
Concrete acceptance.Api refuses redirects, requires trusted HTTPS and suppresses remote error content. Worker gets no appliance credentials and uses validated argv. It keeps one challenged TCP socket and exact EI tuples. PriorityTransition checks lease/key/lockedAt, revision, candidate hash and foreign pending transactions; unconfirmed transaction supplies bounded rollback. Fault helper requires a root-owned non-writable VM marker, boot/PID/executable/cgroup/nonce, pidfd and starttime recheck. Reviewed/fixture-tested only: no fault execution authorized or performed.

Actual independent commands (reviewer-owned snapshot):
```sh
git archive c42307e test/topology/ha-state-sync | tar -x -C /root/.cache/review-r2-final/ha
TMPDIR=/root/.cache/review-r2-final/tmp PYTHONDONTWRITEBYTECODE=1 tools/heavy.sh python3 -m unittest discover -s /root/.cache/review-r2-final/ha/test/topology/ha-state-sync -v
GOMAXPROCS=2 TMPDIR=/root/.cache/review-r2-final/tmp GOTMPDIR=/root/.cache/review-r2-final/tmp tools/heavy.sh go -C /root/.cache/review-r2-final/ha/apps/agent test -p 2 ./internal/descriptors/hasync ./internal/actions/ha-state-sync -count=1 -v
```
Python: Ran 13 tests in 0.034s; OK. Go: hasync 0.049s PASS (3 tests); actions/ha-state-sync 0.096s PASS (5 tests). No skips in these HA suites. Go snapshot archives only 36 needed module package directories plus go.mod/go.sum, selected with go list -deps -test; product files are exact frozen git bytes.
Secret scan: gitleaks detect --no-git --source /root/.cache/review-r2-final/scan --config .github/gitleaks.toml --redact --no-banner: exit0, 7.41 MB, no leaks found across five delta snapshots excluding status documents (status reports scanned separately). Static scan does not invalidate the runtime credential-forwarding finding.
[other: R1/R4/R8] Real two-appliance failover, UDP loss/capture, native IPsec rekey, appliance restart and post-handover fault execution: NOT RUN. Full quick/API controller tests not independently run by R2. No other-aspect grade assigned.
Verdict: BLOCK (1 BLOCKER, 0 MAJOR, 0 MINOR).
