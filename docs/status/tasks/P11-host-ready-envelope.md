# P11-host task envelope

Branch codex/ready-p11-host-20261005; worktree /root/ngfw-wt/ready-p11-host-20261005.
Developer root, daemon-owner none. Fixed existing production packet fixture reserves
slot8; wrapper holds lab shared lock and fixed-fixture lock and rejects collisions.
Own test/topology/ipsec/** and docs/status/tasks/P11-host*. Minimal additional
scope: append full owned-rollback assertions in existing desired/ikev2 integration
packet test; needed to prove cleanup before disposable VPP shutdown. No production
Go implementation changes. Keep shared VPP/config/services untouched.
Native route-based decision supersedes obsolete missing prompt/kernel-vpp checklist.
Independent reviewer upgrade; complete unchanged quick gate required for merge.
