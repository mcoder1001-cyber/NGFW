# F-bruteforce-detectors — R3 contracts/API review

Reviewed exact head: `1df528d4ec77c1e7c9b238b26f34da8cd2e811b0` against `main`. Independent reviewer; no product edits.

Findings: no BLOCKER, MAJOR, MINOR or NIT contract findings.

Existing event enum code 25 and Event.attributes map are unchanged. Added destination_port is a documented optional map attribute, validated as canonical decimal 1..65535 before conversion. SSH/VPN event attributes are unchanged. Port-less legacy scan events are explicitly ignored because they cannot prove distinct-port evidence; updated agents and API agree on destination_port/source_ip/detector. No route/status/pagination/DTO changes, storage migration or contract directory changes.

Executed contract-path check:

```text
git -C /workspace/scratch/e4f791ef53f7/detectors diff --name-only main -- packages/schema packages/proto apps/agent/gen packages/api-client/src/generated
(no output)
```

No schema/proto/generated directory was changed, so no new contract commit or migration is needed. Generated files were not edited by this reviewer; generation/full CI was not rerun or claimed. Existing public messages and service declarations were read directly. Behavioral verification: LDP prospective label-range regression PASS; PIM snapshot-cap preservation regression PASS; detector port-window/event compatibility tests 8/8 PASS (commands/output in R2 reports). Live lab acceptance remains outside R3 verification.

Verdict: **APPROVE**.
