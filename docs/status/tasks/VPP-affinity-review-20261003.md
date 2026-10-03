# VPP permitted CPU selection review

Immutable source `301998cf6fa933130c4c1ea577f8f25032aea5b0` (PR118).
Verdict: APPROVE WITH LIMITS for bounded affinity repair.

The helper reads inherited sched_getaffinity(0), sorts actual permitted CPU IDs and
returns at most the requested 1–8 job count as explicit IDs. It does not assume IDs
are contiguous or start at zero; “1–8” is the job cap, not a CPU-ID restriction.
Invalid job counts and affinity read failure/empty mask fail closed. The build's
taskset applies these IDs to the build child, preserving existing job parallelism
and source-date epoch handling. Neither helper nor build selection changes the
calling shell's affinity or host affinity.

Tests restrict only newly launched Python child processes, derive masks from actual
permitted IDs, and verify a real taskset child's resulting affinity. Live one/eight,
sparse mask, singleton with requested eight, zero and over-cap cases are covered.
If a test host has very few permitted CPUs, the sparse case can degenerate to a
singleton; it still validates permitted-ID selection without inventing unavailable
CPUs. No source/provenance/hash-verification code was removed by the product diff.

Read-only source review, no compilation, tests or active external worktree access.
Coordinator owns fresh-main package/source verification and current-tree CI.
This report does not independently certify the commit message's real build result.
