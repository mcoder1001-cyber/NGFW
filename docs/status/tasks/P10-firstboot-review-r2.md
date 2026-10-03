# Independent P10 firstboot fix verification, round 2

Reviewed developer checkpoint `4222a77f` (frozen reviewer worktree), remote `745348f31c0a4c6048e9a8c13cd8bd0d8169b9f8`. Original review against `54bf1b9e` preserved; reviewer changed only this report.

Completion cleanup MAJOR is resolved: negative marker unit condition removed, bootstrap EnvironmentFile now optional, and script's completed branch becomes reachable automatically without requiring administrator values or rerunning provisioning. Actual fixture simulates completion marker with credentials remaining and checks shipped unit restrictions.

JWT MAJOR remains partially unresolved: assigning random output separately correctly propagates OpenSSL failure, and export now uses the parsed validated key. However command substitution strips trailing newlines. For a valid JWT line followed by an empty duplicate `NGFW_JWT_SECRET=`, sed emits key plus two newlines, Bash strips them, regex accepts the valid key. Systemd instead applies the last empty assignment, leaving runtime key unset and falling back to random signing keys. Explicitly require exactly one JWT assignment line before extraction. Extend tests with valid+empty duplicate in both orders. This is verification of the original stable JWT finding, not a new scope.

Independent reproduction (synthetic zero key only; no real secret):

```text
value=$(printf 'NGFW_JWT_SECRET=%064d\nNGFW_JWT_SECRET=\n' 0 | sed -n 's/^NGFW_JWT_SECRET=//p')
[[ $value =~ ^[a-f0-9]{64}$ ]]
duplicate-empty-ACCEPTED
```

Actual `python3 deploy/debian/ngfw/tests/test_firstboot.py`: 4 tests, 0.691s, OK. Existing duplicate test uses two nonempty keys and misses trailing empty duplicate. No real DB/runtime/systemd acceptance PASS is asserted. R1/R8 remain BLOCK until exact-one validation verified; R2 completion-cleanup finding resolved. Whole P10 remains partial.
