# WAN monitor and bounded LDP foundation integration — 2026-10-03

Branch: `codex/wan-ldp-merge-20261003`.
Worktree: `/root/.codex/worktrees/bbd9/NGFW/.scratch/wan-ldp-integration`.
Base: actual main `cf486eff35e15dcf6fcd8a34711afb2695f03f9d`; its mandatory push CI 37145634983 completed SUCCESS.
Source PR72: `57ebf8f53c47636e6cf5b9b1bd741b9fdb976c92`.
Source PR74: `3437220858d55f86b42665a2417fb830bbccf599`.
Remote/local checkpoint SHAs: pending initial publication; this file is committed with the first port and publication result must be recorded before handoff.

## Ownership and workers

Root owns product changes in this isolated worktree. Three live readonly reviewers: native_ipsec (WAN/RPC/device policy), drift (LDP/FRR/contracts), restart_socket (process/CI/host safety). No reviewer edits product code.
Owned files: agent go.mod; internal/agent/{agent,server,state,projection}.go and rpc_wan*.go; internal/multiwan/{health,probe_linux,runtime}*.go; internal/frrsync/ldp/**; internal/renderers/frr/ldp/**; en/fa multiwan.json; docs/user/network/multi-wan.md; this WIP and envelope. Existing current-main native IPsec, secret persistence, capture ownership, generated contracts and dependency versions remain intact.

## Scope and actual results

Ported original monitor-only runtime, bounded workers and generation drain, durably saved immutable monitor input, WanState owner/filter/error handling and device-pinned probes. Added explicit unavailable results for missing LCP/default namespace/VRF, binding-policy failure, unsupported ICMP IPv6 literal and IPv6-only DNS resolution. HTTP/DNS retain bound IPv4/IPv6 support; real network loss remains packet loss. No unbound fallback or privilege change.
Ported typed all-or-error LDP label translation and bounded FRR renderer/state-reader registration library. No production LDP import/dynamic-source wiring is added.
Initial focused race packages: multiwan PASS 1.804s; frrsync/ldp PASS 1.312s; renderers/frr/ldp PASS 1.160s. Full agent package still running in `/tmp/ngfw-wan-ldp-focused.log`; additional address-family regressions were added after that run started and are NOT covered by these initial passes.

## Remaining work / next command

Run the updated focused tests, complete unchanged local/hosted quick gate on the frozen final current-main head, obtain three independent approvals, archive both original source histories and port checkpoints remotely, then expected-head merge combined PR72. Close PR74 only after its actual inclusion is verified. Check actual main tree and push CI afterwards.
Next command: `GOMAXPROCS=2 GOFLAGS=-p=2 /root/.codex/worktrees/bbd9/NGFW/tools/heavy.sh go -C apps/agent test -race -count=1 ./internal/multiwan ./internal/frrsync/ldp ./internal/renderers/frr/ldp`.

This bounded merge does not complete either feature. WAN route selection, weighted ECMP, NAT cleanup/stickiness, ABF and traffic acceptance remain open. LDP daemon JSON reader, scheduler source, production renderer import, withdraw/hold-down/restart wiring and daemon/browser/live-forwarding acceptance remain open. Board feature states stay running; historical source CI is not fresh-current-main evidence. No host daemon restart/install, kernel module/sysctl/capability changes or new appliance acceptance is claimed.
