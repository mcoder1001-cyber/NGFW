# Bounded TD19 source resume envelope

The build/lab installers still installed container runtimes and a nested hypervisor despite the VMware/native policy. Remove those package groups and the containerlab downloader; retain native iperf3/netperf/tshark/tcpdump/FRR and image-conversion qemu-utils. Preserve reviewed Go/generator trust/digest/refusal behavior. Lab --check-config now reports its native tool scope.

Validation: tests-first published regression, followed by corrected-source12focusedPASS and unchanged45-case strict fixturesPASS/zero skips; ShellCheck/syntax/diff/check gate pass. APT/download commands are blocked in the new control-flow tests. Full hosted quick and independent review remain pending. No real installation/target acceptance; wider safe installer seam, Python/pnpm pins and inventory corrections remain separate TD19 work. Detailed receipts and next commands in resume-td19-source-20261007-wip.md.

Owner authorized branch codex/resume-td19-source-20261007 and worktree /root/ngfw-wt/resume-td19-source-20261007 from origin/main3ddb1680e475e94d43e8036cd3776bc60c87208b.

Own only scripts/20-install-build.sh, scripts/40-install-lab.sh, targeted fixtures docs/status/tasks/TD-19-test-build-preflight.py and TD-19-test-containerlab.py, and this new envelope/WIP. No scripts00/trust/tools-lab/board edits. No installs/downloads, packets/performance or host/service/security changes.

Remove forbidden Docker/containerlab/libvirt/KVM installer dependencies following original TD19 item5 and VMware/no-container rules; preserve native packet tools and existing strict Go/generator pins/refusals. Tests first, commit and publish, then source change commit and publish. Legacy fixture filename remains so unchanged strict runner/workflow continue loading it; obsolete containerlab package tests become meaningful native installer/control-flow tests. Recovered historical evidence remains on remote refs.

Read entirely: AGENTS, 00-CONTEXT, contributing, decision-policy and prompts/fixes/TD-19. Prior published audit remains at2b4cadc0ad6d0c1ef258883caa70918c053dcf39 on codex/resume-host-prereqs-20261007 (ls-remote verified). No lab slot required. Manager owns independent review/final expected-tree gate/integration.

Do not guess Python/pnpm versions or extend into safe-root seam/inventory. Record exact next tasks and incomplete wider TD19 acceptance in WIP. Owner publication/testing rules supersede historical local-only/test-off instructions.
