# Independent runner/routing fixture/tunnel copy follow-up review, 2026-10-04

Reviewer: management_acceptance, read-only in root workspace; no root edits or GitHub comments. Review excludes fixtures authored earlier by this reviewer.

APPROVE runner isolation fix `d44d44eb6efd88754c1964996f7973b16febc57a`. Build mode now writes both production agent and race test binary/build sidecar into the selected initially-empty output directory. Cache mode validates source/binary/agent provenance, snapshots both executables into that output directory, and verifies snapshot hashes before starting tests. Previously reported same-checkout shared-race-binary overwrite risk is resolved for execution; cached input changing during snapshot is rejected. Source cleanliness/frozen build checks, skip rejection, retained outcomes and owned processgroup cleanup remain in place.

APPROVE current root three-file repair diff (reviewed against d44d44eb6):

- `packages/schema/src/domains/group-a.test.ts` retains exact equality and invalid-input assertions, updates ISIS normalized interface object with ipv4/ipv6 true matching routing schema lines667/668, and RIP normalized object with version2 matching literal default at711. No schema implementation or validation behavior changed.
- `apps/web/src/locales/en/tunnels.json` and matching fa file remove VPP branding and developer NGFW_DF6 host-test knobs from the four limitation strings. GTP-U drop default and required live state, unsupported L2TPv3 deletion/recreation/default-underlay restriction, PPPoE session scope and client configuration location, and unavailable 6RD live readback remain explicit. English/Persian content is aligned; keys and UI behavior are unchanged.

`git diff --check` for the three-file repair is clean. Focused tests are manager-owned and were still running at review checkpoint; review does not claim they passed. Complete unchanged quick gate must rerun after the repair. Precise public-digest gitleaks exclusions in d44 are fingerprint-scoped and have a separate independent review document; they are not a broad scanner exemption.
