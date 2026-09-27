# Task: F-rule-expiry — Temporary rules with expiry, owner and ticket fields   (prepend 00-CONTEXT.md)

## Goal
Rule hygiene: an admin can open a rule "until Friday" and it stops matching by itself; every rule can say who asked for it and why.

## Inputs to read first
- `packages/schema/src/domains/acl.ts`, `nat.ts`, `F-object-model` (schedules), `F-acl`, `F-host-acl-nftables`, alarms.

## Contract changes
Common rule metadata on ACL rules, host (local-in) rules and NAT port-forwards: `expiresAt? (RFC 3339)`, `owner?`, `ticket?`, `comment?`.

## Scope — build exactly this
1. **Enforcement**: at `expiresAt` the agent removes the rule from the dataplane (without a config commit, same mechanism as
   schedules); config keeps the rule marked expired so it can be extended or deleted. Survives agent/box restarts (expired
   rules are never re-installed on resync).
2. **Warnings**: alarm/event N days before expiry (default 3) and at expiry; F-notifications picks them up when merged.
3. **UI**: expiry/owner/ticket columns and filters in the rule editors; "expired" and "expiring soon" chips; extend action.
4. **Tests**: semantic tests (past date rejected on create), agent test with a fake clock (remove at expiry, not re-added
   after restart), topology test: traffic allowed before, dropped after expiry.
5. **Docs**: section in the ACL user doc.

## Acceptance (paste the evidence)
- [ ] Rule with expiry +2 min: traffic passes, then drops at expiry without a commit (pasted)
- [ ] Agent restart after expiry does not re-install the rule (pasted)
- [ ] `tools/ci.sh --base main` green

## Out of scope
Approval workflows (BL-OPS-09), policy analyzer (BL-SEC-04).
