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
sets require a separate supported implementation. Explicit architecture qualifiers
such as `:any` and `:native` are also rejected; no Multi-Arch resolution is claimed.
Inspection uses a private archive snapshot with bounded output, archive count,
sizes and relationship count. Source replacement during inspection is rejected.
Files can change afterwards: reverify the trusted manifest immediately before a
separate installer uses the delivery set.

The plan proves package metadata closure, not that package payloads, maintainer
scripts, application migrations or services work. It does not perform an APT
solver simulation, a clean-machine install or hardware tests. No real complete
delivery set has been validated in this change. Synthetic `dpkg-deb` fixtures
exercise the checker; their VPP boundary stub is not provenance evidence.

For release, build the real product/VPP archives, obtain runtime dependencies
from authenticated repositories, publish the signed APT repository, and run the
existing clean Ubuntu install/remove/reinstall and appliance boot acceptance.
The existing runtime installer and firstboot safeguards still apply. This
checker is a bounded preparation step, not a complete offline installer.
