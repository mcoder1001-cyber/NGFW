# Independent P10 firstboot checkpoint review

Reviewed local `54bf1b9eacd1bc9e08cbd7f7ec0e96750f8b9a9d`, remote `7acf72dd06eb2dfee2c759cf914d47b1c224ebd6`, delta from `940e6d81`. Fresh scope R1/R2/R7/R8; earlier foundation approval does not cover firstboot. Reviewer made no product edits or host installs/service starts.

## Findings

1. **MAJOR — completion marker prevents automatic credential cleanup after crash.** `deploy/systemd/vrx-firstboot.service` uses `ConditionPathExists=!/var/lib/vrx/firstboot-complete`. Script deliberately durably publishes that marker BEFORE removing bootstrap.env. A crash between those operations causes systemd to skip the service on reboot, so its existing completion cleanup branch never runs and plaintext bootstrap credentials persist. Make cleanup-only recovery run when credentials still exist, without reprovisioning completed installations. Test marker-published/credentials-present boundary against the shipped unit start condition.
2. **MAJOR — stable JWT input can be missing while completion succeeds.** `assets/firstboot.sh` validates existing api.env owner/mode, DB line and master-key line, but not JWT. An existing env missing JWT (or with empty JWT) therefore passes and is sealed complete; existing TokensService then uses a fresh per-process key, losing sessions on restart. Additionally `printf ... "$(openssl rand -hex 32)"` can succeed despite the random command failing. Assign/check random output separately, validate exactly one expected 64-hex JWT value in generated/existing env, and export that verified key for bootstrap consistency. Add failures for malformed existing env and random generation while preserving credentials/no completion. Generated secret key length is already checked and should remain checked.

R1/R8 BLOCK until fixed. R2 BLOCK for credential lifecycle finding 1; remaining new SQL/env handling uses fixed identifiers and parses controlled env as data rather than shell code, with sanitized DB failure messages and no secret material in logs. R7 APPROVE checkpoint honesty: actual DB/API/VPP boot explicitly not run, activation/base policy/APT remain unfinished, whole P10 remains running. No new always-pending security boundary decision is identified.

## Independent actual verification

```text
python3 deploy/debian/vrx/tests/test_firstboot.py
Ran 2 tests in 0.363s
OK
python3 deploy/debian/vrx/tests/test_packaging.py
Ran 7 tests in 0.441s
OK
node --check deploy/debian/vrx/assets/bootstrap-db.mjs
exit 0
bash -n deploy/debian/vrx/assets/firstboot.sh
exit 0
tools/ci.sh check --base origin/main
ok: gitleaks — scanned ~40392 bytes (40.39 KB) in 164ms no leaks found
check PASSED (0m02s)
git diff --check
exit 0
```

Firstboot fixtures rewrite only a private test copy's absolute paths and fake postgres/runuser/startup/nginx/service commands. They prove staged failure retention and shell retry control flow, not actual deployed DB/auth bootstrap, systemd ConditionPathExists evaluation, nginx acceptance or VPP rendering. They currently omit both reported failure scenarios.

Application bootstrap correctly reuses deployed API migrations, AuthService seed and existing argon2 verifier; it positively checks usable persisted expected admin before completion instead of treating seed(false) as success. It does not call app.listen. Fixed SQL identifiers avoid bootstrap input injection. Final API deployment/native dependencies and real database execution remain mandatory deferred acceptance. No full quick or real appliance PASS is asserted.
