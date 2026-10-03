# NGFW deployment and tooling rename

Explicit owner-authorized VRX to NGFW technical rename; developer task3.
Branch: codex/ngfw-deploy-tooling-rename-20261003.
Worktree: /root/Documents/Codex/2026-10-03/check-out-latest-code-from-git/work/NGFW-deploy-rename.
Base: 0d174caf96413599a6bae7111bf74d14ecebede1.
Owned: deploy/**, tools/**, .github/**, root scripts/config/package.json/pnpm-workspace.yaml/turbo.json only VRX references; unique task evidence. No pnpm-lock.yaml edits.
Interfaces: NGFW_* environment; ngfw-agent/api/web/meta Debian identifiers;
ngfw-agent/api executable and service names; /etc/ngfw, /var/lib/ngfw,
/run/ngfw/agent.sock; deploy/debian/ngfw; new patched VPP +ngfw version.
Preserve upstream VPP URL/version/tag/commit and real upstream input SHA256.
No host installations, service actions or old artifact provenance relabeling.
Pending CPU118/pipeline120 reviewed patches must be supplied by root before
editing the affected scheduler/gate files. Independent reviews required.

Continuation authorized by root: restore only deploy/strongswan/{README.md,
verify_inputs.py,prepare_stage.py,test_verify_inputs.py,test_prepare_stage.py}
from reviewed 09e16f0e3a0bd5ab340e63710ddba0952eec4a16. Rename nomenclature
without algorithm/gate changes. Unsafe older builder/C source remains excluded.
Whole P11 plugin/build/install acceptance remains unfinished.
