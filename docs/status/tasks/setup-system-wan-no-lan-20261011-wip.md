# Setup WAN defaults and remove LAN step

Branch: codex/setup-system-wan-no-lan-20261011
Published product checkpoint: 2ee48ba98
PR: https://github.com/mcoder1001-cyber/NGFW/pull/227
Base: origin/main 76362bdc7; task commits rebased without conflicts.

Implemented: six-step WAN-only UI; editable defaults from running config and live dataplane interfaces/default routes; ambiguous selection and address-only gateway handling; administrator changes protected from observation refreshes. No LAN creation. Existing LAN, DHCP, NAT, ACL and management settings remain intact. Existing static defaults reused/updated; unedited WAN IPv6, static aliases and DHCP options preserved. Legacy explicit-LAN API callers remain compatible. English/Persian copy, feature specification, browser preview script, getting-started guide and generated API client updated.

Completed validation:
- Final schema setup tests: 20/20 passed on parent tree.
- API setup tests: 17/17 passed; API/schema package lint and typecheck passed.
- Wizard navigation/commit tests: 5/5; WAN inference tests: 7/7; regression tests: 8/8 passed on focused rerun after an initial shared-host lazy-loading timeout. Total 20 UI tests.
- Final web typecheck and focused web lint passed; browser script syntax and git whitespace checks passed.
- API client generation: all 9 scheduler tasks successful with concurrency=1. Failed earlier proto generation fully restored; generated diff is only the six optional legacy LAN properties.
- Independent source review: APPROVE after route duplication, ambiguity, initial-loading race and address-only gateway findings were fixed.

Mandatory quick gate: local tools/ci.sh quick --base origin/main running; final hosted quick job also running. Earlier hosted job failed only because the generated client had not yet been committed; corrected in 3c4085aef. No remaining product implementation work is known. Do not merge before the final gate is green.

No live appliance settings were changed. ISP negotiation, packet delivery and confirmed-commit acceptance require an isolated lab and were not run.

Recovery: inspect PR checks and local quick logs under /root/ngfw-wt/logs/ci/NGFW-20261011-073510-493950; fix any real gate failure, update reviewable PR validation, then mark ready when green.
