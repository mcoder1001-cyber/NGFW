# F-capture-trace-host recovery checkpoint

Branch: `codex/capture-host-20261003`. Remote SHA pending first publication.
Implemented HTTP e2e for authenticated lifecycle, binary download, download audit, admin guards, invalid BPF pointer, capture-busy and running-delete refusal. Added genuine VPP persisted-boot recovery integration test. Added live rig acceptance runner checking preflight, HTTP statuses, packets, hash/size, 0600, tcpdump, tmp cleanup, deletion and NRestarts.
Current host: `/run/vpp/api.sock` absent. Hardware acceptance, globals and screenshots not executed.
Actual checks: Python compilation passed. Dependency installation in progress; Go/API checks next.
Next command: `PATH=/workspace/scratch/da15bc9650a7/toolchain/go/bin:/workspace/scratch/da15bc9650a7/toolchain/node_modules/.bin:$PATH tools/ci.sh --base main`.

2026-10-03 checkpoint: local first source SHA `67d78104c3690e517fa911b48de948334b65551e`. Reviewed fixes add exact interface-prefix validation, dedicated VPP unit + shared/dedicated restart counters and 9 fail-closed runner tests. Go focused test and ESLint passed; check PASSED. Full gate failed on incompletely provisioned buf; generated deletions restored. Final report contains explicit NOT RUN acceptance. Next command: rerun unchanged full quick with the stable verified buf executable, then publish same verified public repository via connector.
