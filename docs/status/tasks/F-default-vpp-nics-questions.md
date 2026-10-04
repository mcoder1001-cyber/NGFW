> Historical recovery record from merge `79fff64a` (2026-09-28). Current recovery verification is in `F-default-vpp-nics-wip.md`; historical completion and publication claims do not describe the current main.

# F-default-vpp-nics — open questions

**Resolved by manager D-177:** (1) NICs without a PCI address are correctly treated as "not enumerated" (documented).
(2) Keep `/proc/net/tcp{,6}` management detection with default-route fallback; the read paths are listed in the "For P10"
paragraph of the status file. (3) The UI screenshot is T4's task. The entries below are kept for the record.

1. **NICs without a PCI address (virtio-mmio, USB).** `vppstartup.HostNICs` does not enumerate a netdev whose `device`
   symlink is not a PCI address — they are silently skipped (never DPDK candidates). Host evidence: the reference host
   (`docs/lab/host-ngfw-a.md`) has only PCI vmxnet3 NICs, so none were skipped. Confirm this is the intended behaviour
   (treat as "not enumerated") for future virtio-mmio/USB hosts; if such a NIC should ever be a dataplane candidate, the
   enumeration and the seed need a policy decision.

2. **P10 AD-6 capabilities.** `HostNics` reads `/sys/class/net/*` (device/driver/address/carrier), `/proc/net/route`,
   `/proc/net/ipv6_route` and `/proc/net/tcp{,6}`. If the packaged agent's capability set (P10 AD-6) does not grant read
   of `/proc/net/tcp{,6}` (used to detect the sshd-peer management NIC), the management NIC would only be found via the
   default route / `--mgmt-if`/`--mgmt-pci`. Please confirm the agent may read `/proc/net/tcp{,6}`, or that route-based
   detection is sufficient in the packaged product.

3. **UI screenshot.** Not captured as a PNG (headless host; no slot web server was started in this run). The rendered
   fields are covered by the API e2e state-view assertions and web unit tests. If a PNG is required for sign-off, it
   needs the slot web server up with a fixture document — flagging rather than skipping silently.
