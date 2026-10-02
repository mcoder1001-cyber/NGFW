# TD19 Go module bootstrap source — independent review

2026-10-02. **APPROVE R1/R2/R4/R7/R8** bounded source freeze
be646973db0569b49940707775e7ce087ff5dd9b. Own isolated
NGFW-TD19-go-module-review/task/td19-go-module-review; only this report authored.
No product/fixture authorship. No findings within requested scope.

R1/R2: BASH_SOURCE directory plus pwd-P anchors canonical apps/agent/go.mod
independently of callerCWD/GO_MODULE/GO_VER. Direct module leaf must be regular
non-symlink. Unique numeric directive parsing never evaluates text, rejects
missing/duplicate/malformed/leading whitespace/trailing annotations/read errors.
1.26 normalizes1.26.0; explicit1.26.0 matches. Every unsupported derived version
refuses unless existing reviewed official archive/version pin matches, before
platform/root/APT/network. Existing unset-only SHA check and exact official SHA
remain, no invented hash or version override. Script suffix from PROTOC_GO_VER
onward byte-identical prior source: tool pins/install flags/path/version checks
and archive verification before destructive install unchanged.

Path policy: quoting handles spaces and foreignCWD. Symlink module leaf is
explicitly refused. Directory components are resolved through ordinary trusted
repository paths; no stronger root-owned/no-symlink-parent authority is claimed.
Invoking a script through an outside alias uses that invocation directory, not
a new arbitrary module environment option; absent canonical module fails closed.
Accepted module text still cannot select another archive beyond fixed1.26.0.
R4/R8: developer bootstrap only, no appliance/sharedhost/VPP/cap/privilege behavior
change. Actual no-mutation fixture commands use copied script in private repo,
not real installer/download. Hosted ShellCheck/full gates still required.
R7: three-path delta includes six separately run fixtures/envelope; original36
suite unchanged, no count inflation/fullTD19DONE/labPASS. Envelope honestly keeps
initial sparse missing-fixture failure and later author36PASS recovery attributed,
not treated as this independent broad test execution. Source dependency exception
unchanged; no main/PR86/board/publication mutation by reviewer.

Actual independent execution: six real copied-script tests PASS0.134s; bash -n
PASS. Additional six malformed/control-plane cases (CRLF, duplicate invalid go,
semicolon, NUL, toolchain-only and indented directive) refuse with zero fake
mutation calls; valid tabs and repository path with spaces/foreignCWD accepted
check-config with zero calls. These eight supplemental assertions PASS.
Existing script install suffix identity and whitespace PASS. No redundant original
36 rerun. ShellCheck unavailable locally, NOTPASS; no hosted gates queried.
No actual APT/network/install/daemon/VPP/nft/SSH/host mutation or heavy build.
Final coherent integration/exact hosted gates remain manager-owned before merge.
