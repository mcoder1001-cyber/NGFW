# Installer artifact preflight checkpoint

Branch/worktree/base and owned files: see adjacent envelope.
Local SHA at start: `f6ae6e555ecb1edf5397ff1a8115076ebfc11d38`.
Published initial checkpoint: `02e31ae52` (CLI push succeeded). Final report follows in
the next commit; its remote SHA is reported after successful push and ls-remote.

Required context, contributing policy, decision policy, P10 and TD-19 prompts read.
Remote main verified unchanged; open PR list empty. Main hosted quick run in progress,
previous db46e75 run successful (not proof for this head).
`/srv/ngfw-artifacts` absent. Bundle README absent; authoritative bundle instructions
are `docs/user/install/bundle.md`. Full artifact inventory still in progress.
Board TD-19 trust parking is stale: historical PENDING file points to answered D-238.
No target readiness or installation success claimed.

Actual checks: git ls-remote PASS; source inspection only. No full quick gate run:
it installs dependencies and is outside this read-only preflight authority.
Bounded preflight complete. Installation readiness: **NOT READY**.
Current failure: retained VPP archives rejected by current provenance gate;
no located complete authenticated runtime delivery set.
Exact next read-only command, once the artifact owner supplies a real set:
`python3 deploy/debian/bundle/install.py /restored/delivery --manifest /trusted/bundle-manifest.json`.

## Actual artifact and builder evidence

- `/srv/ngfw-artifacts` does not exist; new worktree has no build artifacts.
- Retained real output directories exist at
  `/root/ngfw-wt/P10/deploy/vpp/.build/out/26.06-release+vrx1` and
  `/root/ngfw-wt/P10b/deploy/vpp/.build/out/26.06-release+vrx1`.
  P10 contains eleven debs plus manifest/checksums/buildinfo/changes; these are
  historical archives, not a verified current release.
- Current canonical command executed:
  `bash deploy/vpp/verify.sh --no-tests --require-files /root/ngfw-wt/P10b/deploy/vpp/.build/out/26.06-release+vrx1 --install-gate`.
  Exit 1: schema `vrx.vpp-debs.manifest/v2` instead of `ngfw.vpp-debs.manifest/v2`;
  stale build-patch sha256/kind; version `26.06-release+vrx1` instead of required
  `26.06-release+ngfw3`. Static VERSION/series/lock/script checks passed. Tests
  were deliberately omitted for this bounded rejection probe; this is not a
  full release-verification pass. Do not rename/relabel the old artifacts.
- `/root/.codex/worktrees/8c19/developers/Packaging-complete/deploy/vpp/.build/out`
  is absent. `/tmp/ngfw-packaging-vpp-build-active-20261004.log` ends at
  compilation step 2479/2922, timestamp October 4 14:42. Earlier offline attempt
  recorded a missing locked meson input. Neither old log proves a live build.
- Process snapshot: no make/ninja/cc1/dpkg/package builder observed. Node PID
  2053997 and esbuild PID 2054034 have elapsed time over four days and cwd
  `/root/NGFW`; they are not evidence of a current VPP or delivery builder.
- Bounded Debian packaging directory searches in P10, P10b and Packaging-complete
  found no product debs. Broad recursive discovery was stopped after becoming
  slow; only this worker's three find PIDs were terminated. No other worker or
  builder was stopped. Discovery is bounded, not a claim that all disks were searched.
- `/tmp/ngfw-helper-fixture-ca68_vvi/delivery` contains synthetic helper-test
  debs including product names. Test fixture packages are not release artifacts.
  Temporary publish manifests and browser dependency archives were also observed;
  neither establishes a complete runtime delivery.

## Exact conditional recipient command

The following paths are the required delivery contract, **not present artifacts**.
After separate authentication of recipient.py's digest and helper-report digest,
and package tar hash/size checking before extraction, preflight on the recipient:

```sh
env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin LC_ALL=C /usr/bin/python3 -I /received/recipient.py /restored/delivery \
  --manifest /trusted/bundle-manifest.json \
  --helpers /received/ngfw-helpers.tar \
  --helper-report /received/helper-report.json \
  --helper-report-sha256 EXPECTED_SHA256_FROM_AUTHENTICATED_CHANNEL
```

Once that passes and the manager releases the installation stage, exact root
install invocation (already authorized targets only):

```sh
sudo env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin LC_ALL=C /usr/bin/python3 -I /received/recipient.py /restored/delivery \
  --manifest /trusted/bundle-manifest.json \
  --helpers /received/ngfw-helpers.tar \
  --helper-report /received/helper-report.json \
  --helper-report-sha256 EXPECTED_SHA256_FROM_AUTHENTICATED_CHANNEL --install
```

No actual safe runnable install command can be populated yet: paths and trusted
digest are unavailable. Do not execute either placeholder command. A trusted
checkout alternative is the canonical install.py command above, adding `--install`
as root for installation. It simulates APT before install, uses verified private
copies and `--no-download --no-remove`, retains existing conffiles and isolates
APT configuration/environment. Maintainer scripts are privileged and installation
can be partial on failure. The online scripts/10-install-runtime.sh runs apt update,
needs NGFW_INSTALL_APPLIANCE=1 and verified NGFW_VPP_ARTIFACTS, and is not an offline
complete product bundle installer.

## Concrete prerequisites and unresolved evidence

1. Fresh Ubuntu **26.04 amd64** (Ubuntu26 shorthand alone is insufficient), root,
   working SSH/console and manager-confirmed network persistence. Python >=3.12
   at /usr/bin/python3, Bash, dpkg/dpkg-deb, APT, Git, patch, coreutils and ordinary
   awk/grep/sed/find; shellcheck used if installed. Writable /var/tmp with room
   for a second archive copy (verifier caps total at 16 GiB); no concurrent dpkg
   administration. Target inventory has NOT RUN; manager reports network returned
   after reboots but SSH actively refused through 15:37:45 UTC.
2. New real VPP +ngfw3 full output, clean committed builder provenance, matching
   patches/locked inputs and successful full --require-files --install-gate.
   Seven shipping runtime archives only enter the install plan; full eleven-file
   build remains necessary for verification.
3. Matching-version ngfw-agent/api/web/meta debs and authenticated Ubuntu26.04
   runtime/transitive dependency closure (FRR/frr-pythontools, Node22, PostgreSQL18,
   Valkey/nginx/TLS, Kea/Unbound/chrony/logging/monitoring plus installer lists).
   No complete closure or APT solver result observed.
4. Current bundle verifier explicitly requires **ngfw-strongswan**. Current
   ngfw-meta only Recommends it, while P10 product-license note says the old
   strongSwan scope is superseded by native route-based IPsec. This is a concrete
   source/profile reconciliation issue: current preflight will reject a set
   omitting that package. Manager must route an applicable reviewed correction
   or provide the approved package; this worker does not edit production files.
5. Separately trusted runtime manifest, authenticated helper archive/report and
   launcher digest, package transport report, signed release publication/trust
   evidence. None located as a real complete release. Hash consistency alone
   cannot authenticate a publisher.
6. D-238 already authorizes exact FRR/NodeSource trust identities; do not request
   that approval again. NodeSource Ubuntu26.04 runtime compatibility remains
   unproved by D-238. Product-license text remains pending in
   docs/decisions/PENDING-P10-product-license.md for authoritative release metadata;
   that note expressly does not park the installed-development-bundle task.
   PENDING-agent-privileges concerns AI workers, not the product agent, and parks
   nothing. Historical CAP_CHOWN/global-writer concerns are not asserted as new
   approvals: current agent unit still lacks CAP_CHOWN and restricts global /etc
   writes; actual firstboot/socket/renderer behavior needs target acceptance or
   a specific current failure and approved fix, not blanket privilege expansion.

No target SSH, package install, network change, reboot, license environment read,
download or production edit performed. Old board snapshots are not readiness
evidence: main f6ae report supersedes them, PR180/193/196/197 are manager-reported
merged, and fresh gh PR list was empty. Current main quick run was in progress
when inspected. Complete quick CI, clean installation, service/login, packet,
repeat-install and reboot acceptance have NOT RUN in this preflight. This report
is a documentation checkpoint, not merge approval or appliance acceptance.
