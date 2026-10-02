# P10 packaging worker envelope

Task P10; manager CI/release delivery. Branch task/P10-packaging-finish-20261002,
worktree NGFW-packaging-finish, starting main 53a43ce5. Host-independent only;
no lab slot acquired. Laboratory acceptance belongs to the central deferred campaign.

Own deploy/debian/vrx/**, deploy/debian/README.md, deploy/apt/**,
scripts/publish-apt.sh, scripts/10-install-runtime.sh, docs/09-os-packages.md,
docs/install/**, deploy/systemd/vrx-*; exclude manager service and P11 files.
No host package installation, service startup, VPP/nft/system config mutation.
Commit coherent code promptly and publish through manager connector.

Implementation checkpoints are partial; no installed-appliance completion claim.
Independent reviewer ci_dag_review verified pnpm production deploy and identified
writable-path/helper-discovery fixes. Latest local commit 4ca988d7 contains both.

Resume: inspect remote task branch first, restore toolchain, run
`python3 deploy/debian/vrx/tests/test_packaging.py` (seven checks), then implement
firstboot with durable DB seed confirmation before removing bootstrap credentials.
Existing API createApp + DB runMigrations + AuthService.seedBootstrapAdmin can be
reused without opening an HTTP listener. Existing main.ts demonstrates boot order.
Package signing key and secret values must never enter commits or logs.
