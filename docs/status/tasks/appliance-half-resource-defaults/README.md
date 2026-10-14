# Historical appliance operation artifacts

These six files preserve the exact bytes published on `codex/appliance-half-resource-defaults` at commit `fc1fcaa4ae0a78b61b3a04442af466deabb1ebb4`. Their original directory was `docs/status/tasks/appliance-half-resource-defaults/`. The recovery keeps that branch and history intact; only the archived filenames gain `.txt`, with mode `0644`.

They document a one-off root maintenance operation on two identified appliances. They are not product tooling, package inputs, a public authentication path, a supported installer, or a safe replay procedure. Text suffixes reduce accidental discovery/import as scripts; they do not prevent an interpreter from intentionally executing the contents. Do not execute or install these archival copies. Any future operation needs a newly reviewed procedure bound to current host identity and configuration, using the existing product safety pipeline.

## Source mapping and integrity

The names below are relative to this directory. Each archived file has the same SHA-256 as its original source blob contents.

| Original name | Archive name | SHA-256 |
| --- | --- | --- |
| `bind-211.py` | `bind-211.py.txt` | `5dc5aa6d4b6d9f5cef17ee2e6d9d11cbd22dcc459ef2c1e99688e845a13ddf41` |
| `build-plan.py` | `build-plan.py.txt` | `0406a8f8b0738803b3edd73a05674977555f2de874251432eb866737e683de37` |
| `maintain.mjs` | `maintain.mjs.txt` | `f256a1d3705c2f3fa86fbb6f25211b1266a1b2003ef1cc6b3075c7887adbc8dd` |
| `performance.py` | `performance.py.txt` | `8adc5c3b97b89cafad1c726a964edb1d22dda671c321cf7a96d20e32e6b42f27` |
| `persist.py` | `persist.py.txt` | `dc057e5128d489be991f180251bca7d8cd5c1d5c6b4607c63c09f972012eadaf` |
| `verify.mjs` | `verify.mjs.txt` | `c5d7a4f8953e8fd73996058c7ccba80c5703939190691f4fdc83908d5c5e3f16` |

The adjacent [status record](../appliance-half-resource-defaults-wip.md) preserves the original operation narrative with a readable allocation summary and distinguishes historical host observations from current GitHub checks. The archived source is intentionally not edited to imply that replay hazards were fixed or retested.

## Audit findings and limits

| Artifact | Recorded purpose | Why it is not a reusable operational tool |
| --- | --- | --- |
| `bind-211.py.txt` | Bind a fixed inventory of 17 DATA NICs using per-device VFIO overrides, protecting the known management NIC and singleton IOMMU groups | No arguments means mutation, with link detach/down and sysfs writes. Host and group identities are hard-coded. Most safety gates use Python `assert`, which optimization can disable. There is no general rollback transaction in this file. |
| `build-plan.py.txt` | Derive two known CPU/RAM/queue plans from a root-protected snapshot | Top-level code writes the plan. Any first argument other than `211` selects the other host's plan; there is no current host identity guard. File and directory trust comes from the original maintenance context. |
| `maintain.mjs.txt` | Snapshot running config or apply the scoped dataplane plan via installed `systemCommit`, revision checks and auditing | Any mode other than `snapshot` selects mutation. Root checks and the service UID switch belong to that specific root maintenance context. Snapshots contain full running configuration and must remain private; stdout includes dataplane/interface configuration. The script is not an HTTP route and must never be exposed as one. |
| `performance.py.txt` | Set available performance governors only where an entire CPU frequency policy belongs to the selected budget | Top-level code writes sysfs. Root/input gates use Python `assert`; it trusts the local configuration and lacks a rollback procedure. |
| `persist.py.txt` | Write fixed appliance sysctl/systemd/binder files, reload systemd and raise the VPP process memlock limit | Top-level code mutates `/etc` and `/usr/local`, starts a service and changes a live process limit. Its acceptance check is merely the existence of any `committed` marker, not a binding to the intended apply, host or plan. It has no explicit root/argument/ownership validation or complete rollback. |
| `verify.mjs.txt` | Read installed interface state and running revision through the local app container | It does not assert 17/seven DATA ports or require a management row; an empty inventory can satisfy its `some` predicates. It creates the installed app context, so it is not treated as a side-effect-free offline check. Its output alone cannot establish full acceptance. |

The archive contains no private snapshots, `plan.json`, credentials, keys, tokens or database contents. The hard-coded private addresses, PCI IDs and startup hashes are historical deployment evidence. Secret scans and source inspection apply to these published bytes only; they do not inspect on-appliance backups or guarantee the absence of every possible secret. The original scripts assume trusted root-owned local state and installed service code. Preserving them is evidence recovery, not a fresh security approval or extension of authorization.

No archive file is referenced by the Debian preparation manifest or shipped product entry points. Recovery verification compares each archived byte sequence to the named Git blob, parses Python without execution, and uses JavaScript syntax-only checks without loading the installed app. Host execution, authenticated browser, reboot and traffic acceptance remain separate and unperformed by this recovery.
