# F-images input and acceptance questions
- The prepared signed NGFW APT repository and verified VPP manifest are absent in this workspace environment. A complete image needs a signed dependency pool including cloud-init, kernel, both GRUB targets and dosfstools. The builder will fail closed, rather than use unauthenticated packages.
- Reuse actual merged P14 common at `deploy/image/iso/common/`; no shared edits requested.
- Cloud DPDK defaults remain the P10 safe `dpdk { no-pci }` until the hardware wizard binds NICs.
- All advertised local formats are built together. Which release formats are blocking remains a release decision.
- Manager-owned CI may add shellcheck/image unit tests later; the task provides a Go test module so the unchanged quick gate executes source tests now.
