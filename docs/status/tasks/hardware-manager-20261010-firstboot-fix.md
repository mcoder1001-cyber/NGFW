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

## Agent snapshot restart correction

Root also owns the narrow docs/decisions/LOG.md entry for this integration.
After the guarded three-plugin startup succeeded, actual .211 agent restart
failed with `auto-block cache owner mismatch`. The private cache was an empty
two-byte JSON object, owner absent, zero entries. Accepted RPC requests may omit
owner, but the strict cache loader requires the service owner. Persist the
effective owner only after checkOwner and cloning; keep the caller request
unchanged and retain rejection of explicit foreign RPC and persisted owners.

Actual root tests on published ee20250072938a46407c5ff541e61e1afb7db2d5:

```text
cd apps/agent
go test -count=1 -run 'TestAutoBlock' ./internal/agent
ok ngfw/agent/internal/agent 0.337s
```

Independent R7 exact immutable archive:

```text
env -u NGFW_INTEGRATION go test -mod=readonly -count=1 -run TestAutoBlock -v ./internal/agent
--- PASS: TestAutoBlockOmittedOwnerSurvivesRestart
ok ngfw/agent/internal/agent 0.350s
```

The regression exercises accepted ownerless publication followed by strict
NewService restart, verifies that rejected foreign RPC cannot change the cache,
and refuses foreign persisted ownership. A source merge does not correct the
already installed binary or its legacy empty cache. Live fixed native artifact
deployment, scoped known-empty-cache recovery, real NIC seed/binding and reboot
acceptance remain pending. The overall hardware task is not Done.
