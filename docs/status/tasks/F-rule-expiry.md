# F-rule-expiry — temporary rules with expiry, owner and ticket (cloud session charming-johnson, 2026-09-27)

## What was built
| layer | files | behaviour |
|---|---|---|
| contract | `packages/schema/src/domains/ext/rule-expiry.ts`; proto AclRule/HostRule 11–13, NatStaticMapping 10–12, `ACL_RULE_STATUS_EXPIRED = 5` | `expiresAt` (RFC 3339 + offset), `owner`, `ticket` (one printable line, D-049) on ACL rules, host rules, NAT static mappings; `description` stays the comment |
| agent | `subsystems/ruleexpiry/` (+ `subsystems/rule_expiry.go`), `desired/acl.go`, `renderers/nftables/build.go`, `desired/nat.go`, `nat44ei.go` | expired → not projected (ACL status EXPIRED, `rule.expired` warning); a rendered rule's future expiry is noted; one timer at the earliest instant asks for a resync (no commit) → the re-projection removes it; restart never re-installs; the ACL schedule watcher skips expired rules |
| API | `features/rule-expiry/`, `commit/validation.service.ts`, `commit.service.ts`, `state.controller.ts`, `acl.service.ts` | commit refuses a new/re-dated past expiry (`rule.expires-in-past`; rollback exempt); RULE_EXPIRING (3 days before) / RULE_EXPIRED system events once per rule+date (Valkey marker); drift skips `rule.expired`; live status `expired` |
| web | `firewall/ExpiryChip.tsx`, ACL `RulesTab.tsx`, host ACL table, NAT mappings | "Expires" column: expired / expires soon / date, owner+ticket tooltip; ACL "extend by 7 days" action; forms show the fields (schema) |
| docs | `docs/user/firewall/acl.md` §Temporary rules | |

## Decisions
- **Enforcement by re-projection** (the pattern ACL schedules already use): no timers in descriptors — the projection is the one
  place that decides, so commit, resync and restart agree; the watcher only chooses *when* to re-project.
- **Comment = `description`** (already on every rule) — no second free-text field.
- **"Past date rejected on create"** = a new rule or a changed `expiresAt` at/before now; unchanged expired rules stay
  valid so unrelated commits still work; rollback exempt (restores history as it was).
- **Extend** = +7 days from max(now, current expiry), ACL table only (host/NAT: edit the field in the form).

## Evidence (this session)
- Agent: `go test -race ./...` + golangci-lint clean. `TestACLRuleRemovedAtExpiryWithoutACommit` (fake VPP): rule
  rendered, removed ~1.5 s later by the watcher's resync with no commit, kept in Retrieve with owner/ticket, not
  re-installed after an agent restart. Projection tests with a fixed clock for ACL, host (nftables text), NAT.
- API: `expiry.test.ts` (3); `test/e2e/rule-expiry.e2e.test.ts` on local PostgreSQL 16 + Redis (2): past expiry
  400 with pointer, unchanged expired rule OK, re-date refused, extend OK, rollback OK; events once per rule/date.
- Web: model helpers + ACL screen test (chips, extend PATCH, notice); full web suite green.

## Not done (needs the lab)
Topology test "traffic passes, then drops at expiry" on a slot (acceptance 1–2 on real VPP): the fake-VPP test proves
the mechanism; the host run is the follow-up row F-rule-expiry-host.
