# Host 211 private configuration-backup receipt — 2026-10-10

This is a configuration/network-state backup only, **not a full system or data backup**. Existing root filesystem corruption still blocks installation, reboot and repair without the reviewed offline recovery path. No target backup files, package/configuration/service changes or filesystem repairs were made.

Private controller directory: `/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211`. Directory mode verified 0700; every local artifact, error file and manifest verified 0600. Backup bytes remain private, outside the downloadable candidate bundle and outside Git. This public receipt contains only scope, commands, exits, lengths and hashes; no configuration contents, credentials or environment files.

All remote captures used SSH with `BatchMode=yes`, `StrictHostKeyChecking=yes`, `ConnectTimeout=10`, to the authorized host identified in the task envelope. Commands execute read-only and stream stdout/stderr directly to private controller files. No target temporary or backup file is used.

## Actual capture commands and results

| Controller artifact | Fixed remote command | SSH exit | Bytes | SHA256 |
|---|---|---:|---:|---|
| network-config.tar.gz | `tar --numeric-owner -czf - -C / etc/netplan etc/systemd/network etc/resolv.conf` | 0 | 674 | `61a03ab8a91f07fc0bcd7ac2c29cea644d7343cf74f6c5dfc22a3e622005c38b` |
| config-path-metadata.json | `python3 - (read-only configuration-path metadata collector)` | 0 | 504 | `1bb3d187419ccc3ad10538763d584c4cfcb531eb0f874f8a99a594d2094422ea` |
| addresses.json | `ip -j address show` | 0 | 8492 | `3987fdd223b88a00b6f54a50d0a4720f3157a468e6abc11a755169dc90b78fb1` |
| links.json | `ip -j -d link show` | 0 | 36486 | `7c0d63ca5cc99910abd0500e36beae489aba2fd5b3c4042224be8c4f8fc3a275` |
| routes-ipv4-all.json | `ip -j route show table all` | 0 | 1377 | `3fdcb7d7bdab6802e950f61a71502babc666c5ac9c90ea3b6d4d4dbe41ca17f1` |
| routes-ipv6-all.json | `ip -6 -j route show table all` | 0 | 480 | `b66a3b57c3b96f53d772c61af6e6a8f1efc0e66f686e0564b3f1e582d78987a1` |
| rules-ipv4.json | `ip -j rule show` | 0 | 140 | `e79a76eff86a78de4ce5d38c6277dbafbab227656214e1e116d9ac63597dbb9d` |
| rules-ipv6.json | `ip -6 -j rule show` | 0 | 91 | `78fc521d26d8f02af636ce8e1c74bb782cbfd272b8a81529bea0adb9a68ed0ea` |
| nft-ruleset.txt | `nft list ruleset` | 127 | 0 | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| pci-name-iommu-map.json | `python3 - (read-only PCI/name/driver/IOMMU collector)` | 0 | 7738 | `e094bfab69a7e4a71a7f49507e8758114b9d43d3157f6c4c52516cd097dcaf1b` |
| iptables-save.txt | `if command -v iptables-save >/dev/null 2>&1; then iptables-save; else exit 127; fi` | 0 | 0 | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| ip6tables-save.txt | `if command -v ip6tables-save >/dev/null 2>&1; then ip6tables-save; else exit 127; fi` | 0 | 0 | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |

`network-config.tar.gz` includes only existing `/etc/netplan`, `/etc/systemd/network`, `/etc/resolv.conf`. These three selected paths were verified to exist and be readable before capture; no `/root`, SSH keys, credentials, environment files or full `/etc` tree was copied. Archive validation read all regular members, required the explicit path allowlist, found 5 members, and `gzip -t` exited 0. No configuration/archive contents were emitted.

Collectors capture IPv4/IPv6 addresses, detailed links, both families of all-table routes and rules, and physical/virtual interface name/PCI/driver/IOMMU membership. Exact collector stdin programs and command-status metadata are retained only in the private manifest. Every recorded stdout/stderr artifact hash was independently read back and matched after capture.

## Completeness and explicit gap

11 of 12 requested/fallback capture commands exited 0. All selected configuration paths and requested network/PCI state were read successfully; their capture error files are empty. `nft list ruleset` exited 127 because the existing command is unavailable; its empty stdout is **not evidence of an empty nftables ruleset**. Its 37-byte stderr remains private, SHA256 `34a202d3b08acfa4515a2d62d494eb4f61a9457cf61e46acd89a34ab2e8b35da`.

Existing `iptables-save` and `ip6tables-save` were available; both readonly fallback captures exited 0 with zero stdout/stderr bytes. These fallback results do not prove that native nftables tables are absent. A full nftables snapshot remains an explicit gap to resolve after safe offline recovery and before firewall activation; no tool was installed to hide that gap.

Private manifest: `capture-status.json`; SHA256 `3efddb28c12ba8388366329d5c020de60809ca92620d77124957090962f347f6`. All successful command stderr files hash to the standard empty-file SHA256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`. Failed capture diagnostics remain private and were not printed or committed.

No target installation, firstboot, dataplane binding, routing change, service restart, reboot, mounted-root repair or hardware acceptance ran. A configuration backup does not establish that data are safe to repair or that the corrupted filesystem is healthy.

Independent R7 is assigned to inspect permissions, manifest and public receipt/completeness only; its verdict is recorded separately. Preserve this branch/worktree and the private artifacts for resume.
