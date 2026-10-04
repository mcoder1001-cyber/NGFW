> Historical recovery record from merge `79fff64a` (2026-09-28). Current recovery verification is in `F-default-vpp-nics-wip.md`; historical completion and publication claims do not describe the current main.

# F-default-vpp-nics — manager answer (2026-09-28, D-177)
1. NICs without a PCI address (virtio-mmio, USB): intended — "not enumerated, never a dataplane candidate"; state it in docs/user/interfaces/default-dataplane-nics.md. A future policy row can revisit.
2. P10 AD-6: the packaged agent runs as root with CapabilityBoundingSet (no file-capability gate on /proc/net); `/proc/net/tcp{,6}` stay readable under ProtectSystem=strict/ProtectHome. Keep the sshd-peer detection; route-based detection is the documented fallback when `/proc/net/tcp` is unreadable (log a warning, never fail). Add the paths to your "For P10" paragraph so P10 keeps them out of any InaccessiblePaths list.
3. Screenshot: T4 on the integrated main stack after merge (D-175); not owed by you. Finish: commit, `TMPDIR=/tmp/g-w1 tools/ci.sh --base main` tail pasted, 10-line summary.
