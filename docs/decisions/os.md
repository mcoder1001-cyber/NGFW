# Decision: appliance operating system

**Status: OPEN — awaiting the product owner.**

| Option | Pros | Cons |
|---|---|---|
| Ubuntu 24.04 LTS (noble) | FD.io publishes VPP 26.06 packages for it; matches the original plan and docs/09 | not what the dev host runs |
| Ubuntu 26.04 LTS (resolute) | matches the dev host; longer support horizon | no FD.io package build at the time of writing (repo file disabled on the dev host) → VPP must be built from source and packaged by us |

Known facts (2026-09-23): the dev host `172.30.126.195` is Ubuntu 26.04 with no VPP
installed; the product owner has run VPP 26.06 with the DPDK vmxnet3 PMD on a VMware VM
whose OS and install method are not yet recorded here. Record them below when known.

- VM used for the successful VPP 26.06 run: `<ip>` — OS `<version>` — install method `<packagecloud noble | source build | other>`
- Decision: `<24.04 | 26.04>` — date — by
