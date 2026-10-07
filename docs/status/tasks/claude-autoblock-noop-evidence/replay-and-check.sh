#!/bin/bash
# Run one replay attempt with the diagnostic log snapshotter, then verify cleanup.
cd /root/ngfw-wt/claude-autoblock-noop || exit 9
n=$1
echo "START $(date -u +%FT%TZ) $(systemctl show vpp -p MainPID -p NRestarts | tr "\n" " ")"
.scratch/diag-watch.sh $n >/dev/null 2>&1 &
timeout 1100 python3 .scratch/mpls-replay.py $n > .scratch/launcher$n.out 2>&1; rc=$?; cat .scratch/launcher$n.out
touch .scratch/diag$n/stop; sleep 1
echo "END $(date -u +%FT%TZ) rc=$rc"
L=.scratch/mpls-srv6-replay$n/actual.log
echo "--- markers"; grep -aE "^[A-Z0-9_]+=PASS|PASS routes|SHARED_VPP|OWNED_VALKEY|DISPOSABLE_VPP|Error|error:" $L | cut -c1-200
echo "--- 504 count (runner log, api log)"; grep -ac "504" $L; grep -acE "\b504\b" .scratch/diag$n/api.log 2>/dev/null
echo "--- UNEXPECTED_HTTP count"; grep -ac UNEXPECTED_HTTP_RESPONSE $L
grep -a UNEXPECTED_HTTP_RESPONSE $L | grep -av "agent.sock" | cut -c1-300
echo "--- cleanup"; ip netns list | grep -i w14 || echo "no w14 netns"
pgrep -af "vpp -c .*isolated-vpp" || echo "no disposable vpp"
pgrep -af "claude-autoblock-noop/apps" || echo "no owned agent/api"
systemctl show vpp -p MainPID -p NRestarts | tr "\n" " "; echo
flock -n /run/lock/ngfw-acceptance-slot14.lock true && echo "slot14 lock free"
