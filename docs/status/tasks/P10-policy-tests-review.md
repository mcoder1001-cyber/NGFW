# Independent runtime policy boundary tests review

Reviewed test-only checkpoint `3c0e989b` in isolated worktree. Product logic remains unchanged; sole commit change is new test_runtime_policy_boundaries.py. Verdict APPROVE test scope; no assertions weakened.

Actual `python3 deploy/debian/vrx/tests/test_runtime_policy_boundaries.py`: 3 tests in 0.727s, OK. Tests execute the shipped installer logic with private copies of root precondition/absolute paths and fixture artifact verifier/commands. They verify failed guard rename retains original and protected backup without APT, concurrent policy replacement is never overwritten by cleanup, and stale prepublication recovery refuses mutation without rewriting old evidence. Real file operations/metadata comparisons occur in private temporary directories; no host policy or live APT/systemctl runs.

Rename failure is injected at the actual mv command boundary, not patched into production logic; concurrent replacement occurs through fixture APT. These are process/file control-flow tests, not machine power-loss or real package lifecycle proof. Original root checks and artifact gate remain in shipped source. Full final integration CI remains required.
