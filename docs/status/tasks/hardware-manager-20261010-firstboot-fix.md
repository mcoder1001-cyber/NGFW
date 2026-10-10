# Source correction DONE — 2026-10-10 14:12 UTC

PR225 merged d1f3f19d4837de3f7a36bfffcbd3c70593bea307; one final commit3b61 on freshbd25, exact reviewed tree507477. Required final unchanged hosted quick38056374923 SUCCESS and postmerge mainquick38057527122 SUCCESS; packaging gates SUCCESS, independent source/final-delta review APPROVE. Historical321G304failure was fixed in the test, not waived. Reviewed predecessor histories preserved on remote archive refs. Native ee202 artifacts passed all41 packaging fixtures and independent fullarchive/12maintscript/reproduced-agent review; installedfixeda909 on both hosts.

This closes only the appliance source prerequisite; the one hardware campaign remains RUNNING until physical/reboot acceptance. Previous pending statements below describe historical checkpoints.

---

# Appliance firstboot policy-plugin correction

This is a prerequisite of the one owner-requested hardware installation task,
not a separate feature campaign. Actual .211 first API seed failed at revision0:
agent validation reported an unavailable LCP API while the plugin files existed.
Upstream disables linux_cp/linux_nl/npt66 by default. Empty bootstrap input did
not enable them, so installation alone could not supply the required capability.

The shipped fixed initial-dataplane.json enables exactly the three D-060 plugins.
Firstboot installs it0600 and uses the unchanged host-validated startup generator.
Empty PCI configuration still emits no-pci and management blacklisting. The meta
package manifest includes the asset; fixture provisioning redirects its path.
Missing LCP is refused before startup publication/completion. Existing completed
appliances require their separately guarded product startup transaction; a source
merge alone does not change an installed appliance or its database revision.

Decision trace: implement the existing D-060 appliance dependencies in explicit
bootstrap input. Alternatives were injecting plugins globally into the renderer
or disabling base policy. Explicit input preserves D-084 authoritative plugin
configuration and leaves later user startup choices and privilege boundaries
unchanged. No schema, socket/auth/capability or VPP version change is introduced.

Actual focused tests on product checkpoint b1f5bea6bb8a7964165b9ba314d09ecbd238750a:

```text
cd apps/agent
GOMAXPROCS=4 go test -count=1 ./cmd/ngfw-startupgen
ok ngfw/agent/cmd/ngfw-startupgen 0.272s

python3 deploy/debian/ngfw/tests/test_firstboot.py
Ran 6 tests in 20.220s
OK
```

Independent R7 copied that exact git archive into its own private scratch:

```text
go test -mod=readonly -count=1 -run TestApplianceFirstbootPolicyPluginsWithoutPCI -v ./cmd/ngfw-startupgen
--- PASS: TestApplianceFirstbootPolicyPluginsWithoutPCI (0.03s)
ok ngfw/agent/cmd/ngfw-startupgen 0.073s

python3 deploy/debian/ngfw/tests/test_firstboot.py -v
Ran 6 tests in 20.635s
OK
```

The regression consumes the actual shipped document through the real generator
without a current startup. It verifies all three dependencies, no physical PCI
claim, management blacklist, and refusal when the installed LCP plugin is absent.
R7 source correctness/management review APPROVE, durable checkpoint
6c1b1370d04fd0b0f706be05cde0db5a6df459ba. Full mandatory quick and final integration
are still pending; this document does not claim final task or release completion.

Original manager history is preserved on its published operational branch.
Three historical generic-api-key findings were scanner matches on prose, not
credentials; corrected wording is retained without any scanner/config exception.
Final D112 integration imports only the narrow corrected product/setup/report
diff onto current main, preserving the full operational history independently.
