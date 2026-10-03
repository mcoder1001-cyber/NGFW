# PR title

feat(packaging): deliver trusted recipient helpers without a checkout

# PR description

Recipients currently need a source checkout to rerun package/VPP verification or installation after transporting the Debian delivery tar. Add a separate helper tar generated from byte-identical committed canonical installer/verifier files and their actual Python/Bash dependency closure. An independently authenticated standalone launcher checks an externally trusted report digest, checks the complete helper tar digest before parsing, and checks every member against that authenticated inventory before executing a private copy of the existing installer. Package archives never supply executable helper authority; the externally trusted runtime manifest remains mandatory.

The existing full VPP verifier, install gate and all its tests remain unchanged. The isolated environment supplies a fixed `/nonexistent` HOME because the preserved VPP path-guard tests require HOME under `set -u`. Canonical helper delivery explicitly requires Python >=3.12 and normal Debian system Git/APT/patch/coreutils tools. No host installation, VPP/service/privilege changes, Python package dependency, source build or vendored implementation is added. The existing hosted export fixture entrypoint includes standalone tests without changing the workflow or skipping gates.

Validation:

- Real outside-checkout fixture run: 7 tests passed in 104.695s, including read-only preflight through delivered helpers and the unchanged full VPP gate (66 VPP tests), with no gate stub and no host installation.
- Existing installer: 11 tests passed in 51.028s. Existing bundle verifier: 23 tests passed in 44.181s.
- Final combined existing export and standalone fixture entrypoint: 18 tests passed in 167.532s on frozen product `be337b42`, including exact regular tar types, every directory at 0700 under umask 022, and ignored poisoned helper code in the delivery directory.
- Adversarial modified helper tar/report, changed runtime manifest, omitted dependencies (including a deliberately reauthorized incomplete inventory), extra/duplicate/link/traversal/changed/mode-escaping members, nonregular/oversized input refuse. Final strict-REGTYPE tests additionally cover FIFO and historical contiguous member types.
- Syntax and `tools/ci.sh check --base 71cee90b2a28421480dfd67a6d64ca7f47f37200` passed. Initial AND final frozen-product unchanged complete quick gates FAILED an existing Go test outside this branch's file ownership: `TestHostServicesApplyRetrieveRollback`, `rpc_dns_test.go:61`, missing `unbound/unbound.conf`. An independent `go test -race -count=1 ./internal/agent -run '^TestHostServicesApplyRetrieveRollback$'` reproduced the same failure (0.543s). No Go files changed relative to the frozen base; no failure waiver or CI weakening. Final complete quick log: `/tmp/p10-standalone-quick-final.log`; detailed agent log: `/root/ngfw-wt/logs/ci/NGFW-p10-standalone-20261003-131131-195997/09-agent.log`. Unchanged hosted quick and provisioning fixture gates are requested on PR #105's exact final published head; neither pending nor failed gates are passes. This genuine out-of-scope failure requires a separately scoped correction before merge.

This branch starts from PR #98's frozen exporter tree; integrate/review the parent first and verify the final integration tree against current main. Independent review is required; the developer does not self-review or merge.

Synthetic empty Debian payloads prove bounded helper transport and metadata consistency, not upstream build provenance. Bootstrap signing/distribution ownership and security provenance policy, genuine complete product/VPP artifacts, signed release, clean Ubuntu install/remove/reinstall, firstboot and hardware acceptance remain explicit P10 release gaps. This closes the bounded recipient checkout gap only and does not mark all of P10 complete.
