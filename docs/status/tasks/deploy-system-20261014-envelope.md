# Complete remote system deployment

Owner request 2026-10-14: finish the existing deployment branch and merge; build packages, transfer to another machine, install/start the system and perform initial configuration.

Branch: claude/status-check-report-aa44ab. Base main: 4f70b3315b9641738f9d193bbad6fb271c9a3d3f. Original deployment checkpoint: 9b2656215004d2e9ecae373a81fec500c8094021; retained in branch history.

Owned scope: deploy/deploy-hosts.sh, deploy/remote/**, deployment docs, applicable fixture workflow and these task records. Parent integrates; independent builder and initial-setup workers own separate modules/tests. Independent reviewer does not edit product code. No live host was specified or accessed.

Reuse pinned VPP build/verification, canonical runtime installer, Debian preparation, firstboot and authenticated Setup API. Fresh installation is explicit; preserve management, credentials and existing installations. Do not claim production-release certification or native acceptance from fixtures. Mandatory hosted quick and deployment fixtures must pass before expected-head squash merge; verify main CI afterwards.

## Recovery worker assignment, 2026-10-14

Resume on `codex/finish-deploy-20261014`, isolated worktree
`/workspace/scratch/8d2d1b380cba/deploy-work`, from source checkpoint `f9f24c52`.
Worker scope is the existing installed-appliance SSH upgrade path, its focused
fixture suite under `deploy/debian/ngfw/tests`, operator documentation and task
records. No fresh-machine setup or live host access is authorized by this worker
assignment. The parent manager owns independent reviews, hosted gates and final
integration; original source history must remain recoverable.
