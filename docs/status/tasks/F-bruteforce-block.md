# F-bruteforce-block — brute-force / scan auto-block

Merged in PR #49 (2026-09-27). Protects the management/control planes by temporarily blocking a source IP once it
crosses a rate threshold, feeding the block into the Global Blocking engine as a system-owned, TTL'd entry (no config
commit per block).

## Delivered (in-container)

- **Contract**: new `security` root domain — `security.autoBlock { enabled, rules[], allowlist[], maxEntries }` with a
  per-source detector (`webLogin | ssh | vpnAuth | portScan`), threshold/window, block time with escalation cap.
  Semantic validators (allow-list well-formed / non-overlapping; enabled-but-watches-nothing). Proto field 14
  `SecurityConfig`/`AutoBlock` (blocked entries are runtime state → no proto field). Drift guard + `buf breaking` clean.
- **API** (`apps/api/src/features/auto-block`): pure, unit-tested engine (IP/prefix match, sliding windows,
  escalation); a non-throwing `AuditService.onEntry` hook feeds the `webLogin` detector without touching the login
  path; blocks persist in `auto_block` (migration 0006) with an escalating TTL; allow-list + loopback always win;
  expiry sweep + `security.events` bus topic. Routes `GET /state/auto-block` (readonly), `POST
  /actions/auto-block/{unblock,block}` (admin). e2e test drives real failed logins → block, allow-list, unblock,
  expiry, manual block.
- **Web**: `Firewall › Auto-block` — live blocked set with Unblock / Block-by-hand; thresholds and the allow-list are
  edited in `Config › Security`. i18n en/fa; jsdom tests.
- **Docs**: `docs/user/security/auto-block.md`.

## Deferred → F-bruteforce-block-host (needs lab VPP/host)

- Data-plane enforcement of the auto-block set on VPP ACLs + `nftables` local-in.
- Host detectors: SSH (journald), IKE/EAP auth failures, port-scan (nftables counters) — each calls the same
  `AutoBlockService.observe()` path over the agent bridge.
- Live topology test: 10 bad logins from a client → blocked on local-in and through; expiry → unblocked; allow-listed
  never blocked.

## Notes for the follow-up

- `AutoBlockService.observe(source, kind)` is the single ingestion point; the host detectors just need to reach it (or
  a thin agent→API bridge event) with the offending source and kind.
- The API keeps the authoritative live set (`auto_block` table); enforcement pushes that set to the agent as a
  system-owned Global Blocking list (reuse the F-global-blocking push once it lands host-side).
