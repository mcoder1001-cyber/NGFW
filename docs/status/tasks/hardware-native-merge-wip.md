# Hardware/native merge recovery

Branch: codex/hardware-native-merge-20261003. Base: 6db19a513; newer origin changes must be incorporated before validation/merge.
Reviewed local source checkpoint: 1311382aa. Remote recovery archive: codex/hardware-native-recovery-20261003, commit e87c3d2d10d33841e1d81a22c4cb5f1c91ba90f7. Bundle restores 273 commits and passed gitleaks.
Owned files: staged hardware/native integration delta and missing dependency foundations. Independent reviewers: native_ipsec, drift, restart_socket (read-only).
State: conflicts resolving; NOT merge-ready. Current source must compile, regenerate contracts, pass unchanged complete quick CI and independent integration review before merge. Preserve latest PKI/notifications/license changes and merge board rows only after actual successful merge. Main VPP PID1014 must not be restarted.
Next: resolve remaining index conflicts, run packages/proto/gen.sh and unchanged tools/ci.sh quick --base origin/main via bounded heavy wrapper.
