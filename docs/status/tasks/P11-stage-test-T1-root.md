# Independent T1 probe — P11 real verifier environment

Tester: root; no product edits. Date: 2026-10-03, Asia/Tehran.
P11 stage draft is based on local `2dbdff24`; shared installer remains the unchanged intake base. This probe is a bounded real verification prerequisite, not full CI or build acceptance.

Inside the existing `INSTALL.safe_environment()`:

```text
VERIFY.run(['bash', 'deploy/vpp/verify.sh'])
REAL STATIC VERIFIER REFUSED: InvalidBundle bash failed

subprocess.run(['bash', 'deploy/vpp/verify.sh'], capture_output=True, text=True)
Exit: 1
ok   scripts parse + shellcheck; no .deb/.whl/.build in git; .gitignore covers deploy/vpp/.build and *.deb
FAIL tests/run.sh failed:
verify.sh: 1 finding(s)

subprocess.run(['bash', 'deploy/vpp/tests/run.sh'], capture_output=True, text=True)
Exit: 1
ok 15 - guard: path inside root accepted
ok 16 - guard: root itself accepted as build dir
ok 17 - guard: ../ escape refused
ok 18 - guard: /root/vpp refused
ok 19 - guard: / refused
deploy/vpp/tests/run.sh: line 60: HOME: unbound variable
```

The real static verifier failed twice, then the direct unchanged tests localized the failure. This is a real source/environment contract defect: the intended clean helper environment does not supply the fixed HOME required by the mandatory verifier tests. It is not missing lab access. Stubbed positive intake fixtures plus real negative provenance refusal cannot prove the valid path works.

Remedy: supply a fixed nonexistent HOME inside the sanitized context, preserving the caller-independent PATH/locale and every test. P10's separate standalone branch already implements the corresponding shared-helper correction; coordinate ownership or implement a bounded P11 context fix. Never restore arbitrary caller HOME or skip VPP tests. Add and run a meaningful real static verifier positive check under the clean environment.

Verdict: FAIL for this prerequisite until independently rechecked. No final stage or release approval.

## Independent resolution check, 2026-10-03

Historical failure above retained. Developer supplies fixed HOME=/nonexistent inside existing sanitized snapshot context, never caller HOME. Frozen source41c67eb0e6d847dee59501897b01e5cca5f91b32, tree24838e528c6abba17533238de9b99b5a23bec436, remote0208f06b23c8fee0b1890eb55d5e64a3f9117cf8, PR109.

Root independently reran actual suites:
```text
python3 deploy/strongswan/test_verify_inputs.py
Ran11 tests in96.641s; OK; exit0
python3 deploy/strongswan/test_prepare_stage.py
Ran23 tests in140.207s; OK; exit0
Real mandatory static verifier inside clean verified_snapshot:
ok tests/run.sh:66 passed,0 failed
verify.sh:OK
```
The positive static tests execute original VERIFY.run without stubbing or skipping unchanged VPP66 cases. This resolves the concrete HOME prerequisite defect. Positive build-provenance fixtures still do not establish a genuine product build; stage/full integration and release acceptance remain separate requirements. Raw independent logs stored work/root-p11-intake.log and work/root-p11-stage.log.
