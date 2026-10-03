# Validate an offline Debian delivery set

The read-only checker rejects a delivery set missing the management packages,
FRR, any runtime package in the existing appliance installer, the seven shipping
VPP packages, or a declared dependency. It produces hashes and a relative-file
installation plan. It does not install packages or start services.

Prepare a directory containing:

- `vpp/`: the complete real output from `deploy/vpp/build.sh`, including its
  manifest and checksum files. The existing VPP verifier must accept it with
  `--require-files` and `--install-gate`.
- Product archives: `vrx-agent`, `vrx-api`, `vrx-web`, and `vrx-meta`, all at the
  same version, built from the repository packaging recipe.
- `vrx-strongswan`, the separately built P11 product package with the VPP plugin;
  an upstream strongSwan package cannot replace it. This complete-runtime profile
  requires it even while `vrx-meta` only recommends it.
- Runtime and transitive dependency archives for Ubuntu 26.04 amd64. Include
  FRR and `frr-pythontools`, Node.js 22 and PostgreSQL 18. Existing package
  dependency version constraints must be satisfied. The checker does not assume
  dependencies are already installed on the destination machine.

Run from a source checkout with Python 3, Bash and the Debian package tools:

```sh
python3 deploy/debian/bundle/verify.py /path/to/delivery > bundle-manifest.json
python3 deploy/debian/bundle/verify.py /path/to/delivery --manifest bundle-manifest.json
```

Keep the expected manifest outside the delivery directory and obtain it through
a trusted channel. A matching SHA-256 detects changes relative to that manifest;
it does not authenticate the publisher. Existing signed APT publication remains
the release trust mechanism. Never treat a manifest supplied alongside unknown
packages as approval to install them.

Only `install_files` belong to the runtime plan. Development/debug packages in
the full verified VPP build output are excluded. Paths in the plan are relative
to the delivery root; no APT command is emitted or executed. Duplicate package
names, foreign architectures, symlinked archive paths, unresolved versioned
dependencies, declared conflicts/breaks and unsupported relationship syntax
fail closed. Versioned virtual `Provides` and dependency alternatives are
supported. Architecture/profile-restricted relationships and non-amd64 package
sets require a separate supported implementation. In `Depends` and `Pre-Depends`,
a direct real-package `:any` dependency is supported only when the named package
in this amd64/all bundle declares exactly `Multi-Arch: allowed` and satisfies
its version constraint. `foreign`, `same`, `no`, absent or unknown Multi-Arch
values cannot authorize `:any`. This follows [Debian Policy §5.6.34.4](https://www.debian.org/doc/debian-policy/ch-controlfields.html#multi-arch-allowed).

Qualified virtual dependencies remain unsupported and are rejected even when
another alternative could satisfy the relationship. Unqualified versioned
virtual dependencies retain their existing behavior. `:any` in `Provides`,
`Conflicts` or `Breaks`, explicit foreign/native architecture qualifiers,
`:native`, architecture restrictions and build profiles are rejected. No
cross-architecture installation or general Multi-Arch solver is claimed.
Inspection uses a private archive snapshot with bounded output, archive count,
sizes and relationship count. Source replacement during inspection is rejected.
Files can change afterwards: the installer below takes its own private snapshot
and verifies that snapshot against the trusted manifest before use.

The plan proves package metadata closure, not that package payloads, maintainer
scripts, application migrations or services work. It does not perform an APT
solver simulation, a clean-machine install or hardware tests. No real complete
delivery set has been validated in this change. Synthetic `dpkg-deb` fixtures
exercise the checker; their VPP boundary stub is not provenance evidence.

For release, build the real product/VPP archives, obtain runtime dependencies
from authenticated repositories, publish the signed APT repository, and run the
existing clean Ubuntu install/remove/reinstall and appliance boot acceptance.
The existing runtime installer and firstboot safeguards still apply. This
checker is a bounded preparation step; the explicit installer below still needs
real release artifacts and clean-target acceptance.

## Preflight and explicitly install a trusted delivery set

The installer requires the expected manifest outside the delivery directory.
It copies `.deb` archives and the VPP manifest/checksum files into a private,
bounded snapshot using no-follow file descriptors, then runs the existing full
bundle and VPP verification against that snapshot. Symlinks and special files
are rejected. The source directory can be on removable storage; installation
uses the verified private copies. Temporary copies under `/var/tmp` are removed on success or
failure. Every verification helper also runs with a fixed system PATH and
minimal environment; caller loader, shell startup, proxy and temporary-directory
settings are removed before preflight. Allow space for a second copy of the archives (up to 16 GiB).

The default only prints the verified plan and never runs APT or starts services:

```sh
python3 deploy/debian/bundle/install.py /path/to/delivery --manifest /trusted/bundle-manifest.json
```

Only on an explicitly authorized fresh Ubuntu 26.04 amd64 target, request the
mutating operation:

```sh
sudo python3 deploy/debian/bundle/install.py /path/to/delivery --manifest /trusted/bundle-manifest.json --install
```

This requires root, checks the OS/release and dpkg architecture, and first runs
an APT simulation. If simulation fails, installation does not run. Both commands
use the exact verified local archive paths, `--no-download`, `--no-remove`, an
empty private repository list, lists/cache directories, isolated APT
configuration and a minimal environment with no inherited proxy settings.
It does not run `apt update`, permit downgrades, add repositories or install
recommended/suggested packages outside the complete supplied runtime profile.
Existing configuration files are retained (`--force-confold`). APT still reads
the target's installed-package status and uses its normal dpkg database/lock.
Archives remain root-private, so APT reads them as root rather than weakening
the snapshot permissions for its `_apt` helper.

**Installation is a privileged mutation.** Trusted package maintainer scripts
can write configuration, migrate data and start services. The installer does
not sandbox those scripts or prohibit their own network activity. The isolated
APT acquisition path does not prove that every package script is offline.
Installation is not transactional: a failure may leave partially configured
packages; retain the APT/dpkg output and inspect the target before retrying.
Concurrent administrative package changes must be avoided. Use this operation
for fresh targets; it is not a validated upgrade or rollback mechanism.

Synthetic tests build real small `dpkg-deb` archives, stub only the VPP provenance
boundary and capture APT commands without executing them. They prove command
construction and rejection paths, not real artifact provenance or installation.
A genuine complete bundle, signed-release trust, actual clean Ubuntu
installation/remove/reinstall, firstboot and hardware validation remain required.
The bounded verifier's unsupported relationship/Multi-Arch syntax limitations
above still apply and can reject real distribution package sets.
