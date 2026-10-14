# Open-work recovery envelope

- Request: list unfinished NGFW work, inspect all open branches, and complete them.
- Integration branch: `codex/finish-open-work-20261014`; base `4f70b3315b9641738f9d193bbad6fb271c9a3d3f`.
- Manager owns: central acceptance ledger, dated audit/closeout, this envelope, and the packaging workflow path trigger.
- Deployment worker: isolated `codex/finish-deploy-20261014`, recovering `claude/status-check-report-aa44ab` at `f9f24c52baddc3c59d5598ce456685e7fd3eec00`; owns installed-host deployment script, focused packaging tests, installation documentation and deployment WIP.
- Operations worker: isolated `codex/finish-resource-records-20261014`; recovers the resource-operation records at `fc1fcaa4ae0a78b61b3a04442af466deabb1ebb4` and Ethernet closeout at `80810da304f9d5eb7b251aafdb3855c7dfe3c592`; owns only their archival/status paths.
- Read-only acceptance auditor checks final receipts against stale ledger entries.
- No host deployment, NIC rebinding, service restart, credential change or new license choice is authorized by this recovery envelope. The work completes source and records; actual target acceptance retains its existing explicit boundaries.
- Preserve original remote branches and checkpoint history. Integrate only unique, reviewed changes on current main, use one final integration commit and expected-head PR merge after mandatory hosted quick and packaging fixtures pass.
- Each source reviewer is independent of its developer. Record applicable correctness, security, evidence and operability verdicts and focused test output. Do not treat archived operation scripts as supported executable tooling.
- Recovery: inspect remote main/PR/checks first, then this integration branch and worker WIP. Source completion, hosted CI and live acceptance must be reported separately.
