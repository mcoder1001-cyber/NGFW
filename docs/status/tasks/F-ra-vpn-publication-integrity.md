# RA publication integrity incident

2026-10-05: connector creation/ref updates succeeded, but batched source retrieval through exec_command was truncated inside JSON string contents. Valid JSON did not establish exact source identity. No secret exposure was observed. Affected published history is invalid for source grading.

| Published checkpoint | Exact local original | Damaged product path |
| --- | --- | --- |
| af6c4dded457ba8e90757bf977e47514ebec03db | 141f61bcd | deploy/debian/ngfw/tests/test_prepare.py |
| 3551e90d33b65f87c5ff697c9b95e5918f889573 | 3d2ae27990 | same test inherited the af6 damage |
| 8b2fb8cb8075fef7d0762f729e33cc648e1c0c2a | archive/ra-local-e71406db4 | apps/agent/internal/ra_vpn/wire_capture_test.go |
| b311ecd651840b965b712166f4a61d53a2c3c6d1 | archive/ra-local-26f671b21 | deploy/ra-vpn/package-engine.py |

Three publication events also omitted the final WIP paragraphs. The exact original evidence is retained in local original commits/archive refs and restored into current status. Product defects were transport corruption, not waived test failures. The repaired staging test combines exact current MAIN hardening/A/B assertions with the original five RA assertions/fixture lines; prepare combines current MAIN with the original three RA staging commands. Cumulative staging verification requires actual current MAIN dependency source.

Audit: grouped every commit subject in this task's HEAD ancestry since base7b507db5 against all matching local/remote Git objects, then compared the complete Git trees (not selected files). Only the four pairs above differed: three product paths and WIP paragraphs. All other matching checkpoint pairs have equal trees. Current owned product directories were additionally searched for the truncation marker. Reviewed source history is retained without rewriting main or removing damaged evidence.

Prevent recurrence: retrieve each file independently with an adequate token budget, refuse truncation markers/warnings, and require create_tree SHA to equal LOCAL HEAD^{tree} before creating any commit or updating a ref. Repaired1e18 tree60c70878b64bdeb295c751205301ac073f008878 was equal locally, but third test damage inherited from af6 was subsequently found independently by R2 and is restored in this checkpoint. Final grading must use the complete subsequent repair, never1e18 alone.
