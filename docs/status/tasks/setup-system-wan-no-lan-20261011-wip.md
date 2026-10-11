# Setup WAN defaults and remove LAN step — validation in progress

Branch: codex/setup-system-wan-no-lan-20261011
Published source checkpoint: ec4cc410cdf6bfc58c777b27df284f8aedb2f544
Draft PR: https://github.com/mcoder1001-cyber/NGFW/pull/227
Base: origin/main 76362bdc7; task changes rebased without conflicts.

Implemented: six-step WAN-only UI; configured/live WAN defaults including address-only gateways; editable values protected from observation refreshes; English/Persian copy; browser preview script and getting-started guide. Contract allows omitted LAN fields with legacy caller compatibility. LAN/DHCP/NAT/ACL/management preserved. Static default routes reused/updated, WAN IPv6 and unchanged aliases/DHCP options preserved.

Passed: schema setup tests 20/20 on final parent tree; helper API tests 17/17, schema/API lint and typecheck; parent focused web lint; browser script syntax and git diff whitespace check; source review APPROVE after reported route/ambiguity/loading fixes. Forbidden-pattern/contract check passed before rebasing.

Initial parent UI run: failed module resolution because API-client had not yet been built; 6 WAN inference tests passed. Dependency now built; final UI/typecheck rerun running. Initial shared proto generation process killed by host; retry using scheduler concurrency=1 running. Generated proto outputs were removed by failed generator and must be fully restored by successful generation before final commit.

Remaining: finish client generation and final UI/typecheck; commit generated API artifacts and updated task prompt; complete mandatory quick gate (hosted job also running), update PR validation and publish final checkpoint. No deployment or live appliance changes. Lab acceptance not run.

Next: poll generation/UI sessions; commit generated outputs only after successful generation; run tools/ci.sh quick --base origin/main with bounded concurrency.
