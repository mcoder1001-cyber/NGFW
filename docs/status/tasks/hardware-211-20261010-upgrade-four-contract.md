# Four-package upgrade preparation; not executed

Worker owns only host211 and its task source/evidence. ROOT must release the
actual phase after applicable review and exact artifact identities. This
procedure upgrades ngfw-agent, ngfw-api, ngfw-web and ngfw-meta from
`0.1.0~dev+2045ab8b3d2f` to `0.1.0~dev+ee2025007293`, compiled from
`ee20250072938a46407c5ff541e61e1afb7db2d5`. It does not substitute the later
integration commit as a build attestation.

`upgrade-four.py` is controller-side. Mode `inspect` is read-only. Mode
`prepare` consumes its private inspect file and SHA, records the originally
absent policy/mask and staged guard identities durably before either rename,
then creates executable policy101 and a persistent VPP mask while the existing
VPP process stays active. Collision or identity drift refuses the phase.
`restore` removes only those exact guards (or accepts their original absence),
reloads units and proves protected state unchanged; it retains the private
recovery record and never starts services. Partial staging failures remain
explicit and require identity-based handling; there is no blind cleanup.

Modes `upload`, `simulate`, `install` additionally require a controller manifest
and its SHA. Manifest schema is `source_sha`, `version`, `packages` with exactly
four unique entries `{Package, file, sha256}`. Files are sibling `.deb` archives;
the helper independently checks every control name/version/architecture and
archive digest. Streaming upload creates only a new private directory in /run
tmpfs, validates the exact four regular members and fsyncs their hashes. The
controller streams upload stdout/stderr directly into private files while
writing tar stdin, so a large early-refusal snapshot cannot block SSH pipes.
manifest must be supplied from the manager's reviewed build receipt; source
preparation alone does not establish archive identities or full maintscript
applicability.

Simulation must contain exactly four Inst lines and zero Remv lines, and only
the named packages. `install` consumes that private simulation file/SHA and
resimulates the same plan before a transaction with policy101, persistent VPP
mask, `DEBIAN_FRONTEND=noninteractive`, `NEEDRESTART_MODE=l`, and supported
`VPP_INSTALL_SKIP_SYSCTL=1`; it refuses detected needrestart hooks, does not
timeout dpkg, preserves existing conffiles, and captures complete private apt
output. Exact four configured versions, fixed agent executable SHA256
`a909ae56ecee2431921659d0d9d489a14768fb12a4d71628627659fecd463781`
and empty successful dpkg audit are required afterward. The manager's compiled
manifest must attest that same executable digest. These controls do not authorize a broader dependency or
OS upgrade.

All modes pin management enp4s0/PCI04/igc/group28 and controller TCP22, all17
original kernel drivers/names/singletons, complete L3, DNS,16 sysctls including
nr_hugepages1024, clean root and ioerr6,21 unit states and execution times,
VPP33868/nginx9281 active with zero restarts, API/agent stopped, exact735B367e
noPCI startup, canonical private three-key api.env, TLS/secret/firstboot and
seed inputs. Native NFT snapshots are retained; comparison ignores only
packet/byte counters, preserving every rule and foreign table. New kernel
storage errors are checked with word-bounded UNC matching. The existing2B
empty auto-block cache remains untouched; its known recovery is ROOT-owned
after the package fix. No firstboot, startup apply, binding, API/database
revision, service activation, reboot, sysctl or credential write occurs here.

Exact next command is `python3 docs/status/tasks/hardware-211-20261010-upgrade-four.py inspect`
ONLY after publication, R7 applicability and ROOT's phase release. Then pass
the private printed file/SHA to `prepare`; provide reviewed manifest/SHA for
`upload` and `simulate`. ROOT must review the actual four-package plan before
`install` receives its simulation path/SHA. Every stdout/stderr receipt is
private0600/fsynced under the owned host211 controller directory and is never
committed. Any actual failure remains a failure; no automatic retry, repair,
cache rewrite or service start is part of this contract.
