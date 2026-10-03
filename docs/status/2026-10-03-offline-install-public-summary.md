# Offline installer increment — 2026-10-03

PR #91 adds trusted-manifest preflight and explicitly requested offline local-package installation. Default mode only prints a verified plan. The mutating mode requires root on Ubuntu 26.04 amd64 and simulates isolated APT before installation from private verified snapshots.

Eleven installer fixtures and thirteen existing verifier fixtures pass with zero skips. Applicable independent reviews approve the bounded product scope. Provisioning fixture CI 37088393019 succeeds; unchanged complete CI 37088392772 is still running at this checkpoint. Merge requires that gate to succeed and the current main integration tree to remain verified.

This increment does not establish a complete real delivery set, signed release publication, Ubuntu 26.04 installation lifecycle, firstboot or hardware acceptance. P10 remains incomplete. Package maintainer scripts remain privileged, can use the network, and installation is not transactional.
