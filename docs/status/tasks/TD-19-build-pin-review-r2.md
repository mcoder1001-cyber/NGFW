# TD-19 build pin narrow verification round

Exact frozen `8d769cb9242aa0cfaa39e407e9096ca547b4fc64`, isolated task/TD19-build-pin-recheck; remote checkpoint reported e0911702 by manager. Initial BLOCK atf7d4f694/ff08de1e remains historical evidence.

**APPROVE this corrected build-pin delta (R1/R2/R5). Previous MAJOR closed.** Installation decision now inspects explicit `/usr/local/go/bin/go`; current and persisted PATH put that directory first, `hash -r` clears shell resolution cache, and selected Go exact version/platform is verified before all three generator installations. Linux+x86_64 refusal is explicit. No checksum/override gate loosened or new privilege added.

New shadow regression extracts the actual script fragment between PATH selection and corepack, substitutes only the installation directory into a temporary fixture, and executes it with earlier old fake Go plus freshly pinned fake Go. It verifies all three generator calls use fresh with exact versions. Wrong fresh version refuses all generator calls. This is meaningful executable consumer-selection proof rather than a source-string assertion alone. Persisted profile prepend is additionally asserted. These fixtures do not prove real installation/network behavior.

Personally ran:

```
python3 docs/status/tasks/TD-19-test-build-preflight.py
Ran5 tests in0.027s — OK
python3 docs/status/tasks/TD-19-test-provision-order.py
Ran3 tests in0.039s — OK
bash -n scripts/20-install-build.sh tools/lab
exit0
```

No actual download, archive extraction, APT/profile mutation, remote apply or host installation executed. This review excludes developer changes after frozen8d769cb9, including containerlab script40. Full TD19 remains incomplete and P10 dependence/source contract exception are unchanged. Full unchanged hosted quick must pass on final integration head before merge.
