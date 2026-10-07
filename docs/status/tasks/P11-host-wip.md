# Final integration checkpoint

Manager-owned codex/integrate-p11-host-20261005, /dev/shm/ngfw-integrate-p11-host-20261005. Pinned final parent 31ad921385c5fe08227bc20f4150eda6a8e1d873 (actual PR174 merge). Full unchanged local and hosted quick gates are pending on this frozen integration tree. Reviewed provisional receipts are archived remotely at codex/archive-P11-host-integration-receipts-20261005 (9c2368c3). Reviewed sourcef59c3ea7 preserved locally and remote codex/archive-P11-host-reviewed-20261005 (081c9ac5). Only reviewed ten-file task delta applied with three-way merge; current main native auth/certificate/runtime fixes preserved. Mandatory R1/R2/R4/R7/R8 APPROVE; independent exact source fullquick9m55 and fresh real both-role T3 PASS107.48s/109.40s with encrypted ICMP/TCP, foreign-SPI refusal, rekey, agent restart, route withdrawal/recovery, default DPD peer crash/recovery and authoritative owned rollback. No certificate/full appliance proof inferred. Final complete integration quick/hosted and expected-head merge remain required.

Current-main live acceptance on0730017ca independently of old artifacts passed bothroles107.542s/109.523s; safe new summary and manager actual report adjacent. Fresh independent current-main T3 remote52fd2795 PASS107.94s/109.55s with both current native authpatches; report+safe receipts adjacent. Full final local/hosted gate still required. Historical evidence follows.

# P11-host recovery

Branch codex/ready-p11-host-20261005, worktree /root/ngfw-wt/ready-p11-host-20261005.
Root developer. Fixed existing packet fixture slot8, daemon-owner none; shared
lab lock and fixture lock, collision preflight, disposable VPP only.
Published wrapper checkpoint4ecec8238606903591315a82dbaf672030672867.

Initial production agent peer-loss campaign PASS both responder/initiator on
source875cc6dc. Shared VPP MainPID1014 and NRestarts0 unchanged. Summary
`.scratch/P11-host-20261005-060224-1099176/summary.json`; captures private0600.
Added actual full owned rollback before disposable VPP shutdown. First run26cd
FAIL because empty DesiredState with omitted authoritative subsystems means no
applicable domains (service.go authoritative contract), not removal. Corrected
request explicitly selects vpn/tunnels/routing/interfaces/vrfs (26ebf631).
Current full --peer-loss rerun running; log `.p11-host-final-run.log`. No new PASS
claimed until exact wrapper summary confirms both modes and owned cleanup.
Next: read new `.scratch/P11-host-*/summary.json`, then unchanged complete quick
and independent upgrade-agent review. Raw SA/peer/API logs remain private; only
nonsecret booleans/counters/SHA summaries may be committed. No target deployment.
