#!/bin/bash
# Diagnostic only: snapshot slot14 service logs while a replay runs (the runner deletes them on exit).
d=/root/ngfw-wt/claude-autoblock-noop/.scratch/diag$1; mkdir -p $d
while [ ! -e $d/stop ]; do for f in api.log agent.log; do [ -s $(ls -dt /root/ngfw-wt/claude-autoblock-noop/.scratch/isolated-vpp-*/test-run/w14 2>/dev/null | head -1)/$f ] && cp $(ls -dt /root/ngfw-wt/claude-autoblock-noop/.scratch/isolated-vpp-*/test-run/w14 2>/dev/null | head -1)/$f $d/$f; done; sleep 0.3; done
