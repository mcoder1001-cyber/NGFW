# Independent P10 runtime installer review

Reviewed frozen local `9a0bff63`, remote `c1aae3d3baaee92ce53e4c94b2d30934ab6a915a`; scope scripts/10-install-runtime.sh, runtime docs and task evidence. Reviewer wrote only this report; no live installation or service action.

**MAJOR — runtime package installation can start services before appliance provisioning.** The installer calls apt-get install with verified VPP and daemon packages before firstboot drop-ins or a no-start maintainer-script policy is installed. Distribution package postinst may start VPP/nginx/nftables with default config at that point; disabling selected daemons after installation is too late, and VPP is not in that list. Explicit appliance opt-in prevents accidental shared-host invocation but does not enforce product firstboot ordering. Install a temporary policy-rc.d no-start guard before package mutations, preserving/restoring an existing policy on success/failure, or require an equivalently preprovisioned supported guard. Add fixture evidence that an installation attempt cannot start services. No changes to shared host are authorized by this finding.

R1/R8 BLOCK pending fix; R2 no other new injection/secrets findings. Inputs are quoted, VPP artifacts pass existing file/install validator and only seven verified ship entries become local .deb arguments. There is no FD.io/upstream package fallback, kea-ctrl-agent is removed and rsyslog TLS driver now matches renderer ossl. R7 documents the deliberate appliance-only compatibility change and unfinished P10 release prerequisites honestly.

Actual independent fail-closed checks ran installer with apt-get replaced by a private temporary stub:

- missing opt-in: rejected before apt command;
- missing artifact variable: rejected before apt command;
- nonexistent artifact path: rejected before apt command;
- empty artifact directory/missing manifest: existing verifier exited 1 before mutating apt update/install/purge/autoremove.

The actual VPP validator runs its own read-only `apt-get install -s -qq` checks; these reached the stub for two validation simulations. They are not package mutations. Validator reported `66 passed, 0 failed`, then `FAIL output: cannot read .../manifest.json` and `verify.sh: 1 finding(s)`. No full valid-install path was executed.

`bash -n scripts/10-install-runtime.sh` exited 0. Artifact authenticity, installation lifecycle, service ordering and whole appliance acceptance are not inferred from rejection tests. Full hosted quick and later real acceptance remain mandatory; lab not run is not PASS.
