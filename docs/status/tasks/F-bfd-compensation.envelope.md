# BFD second-round recovery repair envelope

Role: developer (prior independent R8 verdict applies only to old source).
Branch: codex/bfd-compensation-20261005.
Worktree: /root/ngfw-wt/bfd-compensation-20261005.
Base: 686846c50a03315a4df87c214779aa884554e612.
Owned: apps/agent/internal/descriptors/bfd/{bfd.go,ownership.go,recovery.go,recovery_test.go,multihop.go,multihop_test.go}; apps/agent/internal/agent/{rpc_bfd.go,rpc_bfd_test.go}; apps/web/src/App.test.tsx and apps/web/src/domains/routing/ospf/OspfPage.test.tsx (root-authorized nav regression corrections); docs/status/tasks/F-bfd-compensation* only.
Task: implement R1 failed-create compensation contract and safe durable successful-add recovery, plus bounded FRR observed timer conversion. Do not change shared host or main; no source edits in ready-bfd worktree while root gate runs.
Publication: separate checkpoint branch; root archives reviewed source and integrates into PR175 later. No independent approval claimed for authored repair.
