# F-management-ui-host source completion

Branch codex/ready-management-ui-host-20261004; worktree /root/.codex/worktrees/f796/work-management-ui-host.
Reviewed published checkpoint36c58acab6d6fee804285e294668340be5b0b040 (PR163).
Real isolated slot TLS acceptance driver: temporary cert/key secrets, committed TLS1.3 rotation,
fresh handshake/fingerprint/revision, TLS1.2 rejection, mismatch400 privateKeyRef pointer,
public API/audit key scrub, unchanged API PID/start time, original certificate rollback and secret
cleanup. Dedicated API-key lock and revision guards preserve concurrent/ambiguous applied state.
Five helper/refusal tests PASS, including a transient real localhost TLS handshake. These are
source helper evidence; they are not product API acceptance. Root+igp independent source APPROVE.
Complete exact-current-main quick gate remains mandatory after root prerequisite baseline repair.

Real product TLS/API, browser/screenshot/nav/tab and API-log scrub acceptance **NOTRUN**,
pending isolated slot stack/baseline TLS/dedicated API key. No fixture or skip is presented as PASS.
Product secure TLS lifecycle already integrated at15ce0d28b (#147).
