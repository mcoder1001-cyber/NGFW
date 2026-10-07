#!/bin/bash
# Run replays sequentially until 3 consecutive PASS or max attempt reached.
cd /root/ngfw-wt/claude-autoblock-noop
streak=0
for n in $(seq $1 $2); do
  .scratch/replay-and-check.sh $n > .scratch/check$n.txt 2>&1
  if grep -q "^MPLS_SRV6_REPLAY${n}_EXIT=0" .scratch/check$n.txt && grep -aq "^MPLS_SRV6_REAL_API_LIFECYCLE=PASS" .scratch/mpls-srv6-replay$n/actual.log; then streak=$((streak+1)); else streak=0; fi
  echo "attempt $n streak $streak" >> .scratch/series.log
  [ $streak -ge 3 ] && break
done
echo DONE >> .scratch/series.log
