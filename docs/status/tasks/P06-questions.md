# P06 — questions / items for the manager

None of these blocks P06; each has a default that is implemented. Numbered for the LOG.

1. **State RPCs missing in the agent contract (routes, neighbours).** `vrx.v1.Dataplane` has no FIB / IP-neighbour dump.
   Implemented: `GET /api/v1/state/routes` pages the connected + static routes the agent *retrieves from VPP* for its
   owner (Retrieve of `interfaces` + `routing`); `GET /api/v1/state/neighbors` answers 501 with the reason. Proposal:
   an additive `contract/…` branch (owner P03b/P05) adding `rpc DumpRoutes(DumpRoutesRequest{vrf, prefix, page_token,
   page_size}) returns (DumpRoutesResponse)` and `rpc DumpNeighbors(...)`. Options: (a) additive RPCs (recommended — BGP
   tables are large, must be paged at the agent), (b) keep the Retrieve-derived view (no learned routes), (c) route
   state through StreamStats (wrong shape).

2. **Clearing a password hash through the config.** D-046 says merge-patch `null` clears a write-only member; with
   RFC 7386 a `null` deletes the member, which is indistinguishable from "absent = keep". Implemented: absent/null =
   keep (safe side); a user can be disabled (`disabled: true`) or removed. Options: (a) keep (documented),
   (b) a dedicated `DELETE /api/v1/auth/users/{name}/password` admin action, (c) a sentinel value in the schema
   (contract change). Recommendation (a) now, (b) with the users UI (P07b).

3. **Users: `management.users` ↔ `app_user`.** Implemented: on every promoted revision `app_user` follows
   `management.users` (role, disabled, hash when given); users that came from the config and disappeared are deleted;
   the bootstrap admin (source `bootstrap`) is never deleted by a config commit, only updated if listed. Hashes live in
   `app_user` only; revisions/diff/audit/GET are redacted (D-046/D-070); before validation the stored hashes are
   hydrated back in so `management.admin-exists` sees the truth. Question: should a config commit that omits the
   bootstrap admin *disable* it (stricter) instead of keeping it (no lock-out risk)? Default: keep.

4. **Operator vs users/AAA.** "operator: no user/AAA changes" is enforced on the *content*: any edit, import or
   rollback whose result changes `/management/users` or `/management/aaa` is 403 for a non-admin (checked on the
   whole document, so PUT/import of the root cannot bypass it). Secrets (`/api/v1/secrets`) are allowed for
   operators (VPN PSKs are operator work). Confirm or restrict secrets to admin.

5. **JWT role staleness.** Access tokens carry the role and live 15 min; a role change / disable through the config
   takes effect on the next refresh (refresh re-reads the user) — at most 15 min later. Options: (a) keep (standard),
   (b) per-request user lookup (one DB query per request), (c) shorter access TTL. Default (a).

6. **Agent subsystems not implemented yet.** The API sends `subsystems = HealthResponse.subsystems ∩ ROOT_KEYS`
   (proto.md §2: naming an unimplemented key would be UNIMPLEMENTED). Domains the agent build does not implement are
   stored in running but not applied; every commit/validate response lists them in `notApplied`. Confirm that this is
   the wanted behaviour while factories land (alternative: refuse commits that change a non-implemented domain).

7. **Redocly rule.** `packages/api-client/redocly.yaml` extends `recommended` and switches off only
   `operation-4xx-response` (GET /health and POST /auth/logout have no client-error outcome). Everything else passes
   with 0 warnings.

8. **Agent integration e2e not executed.** `apps/api/test/integration/agent.int.test.ts` implements the prompt's
   real-agent scenario (loop1xx + 10.<slot>.101.0/24; commit → Retrieve + `vppctl show int addr`; rollback; confirm=5
   revert; overlap 400; readonly 403) and skips without `VRX_INTEGRATION=1` + an agent binary (`VRX_AGENT_BIN` or
   `apps/agent/bin/vrx-agent`). P05's CLI flags are unknown to P06: the test passes `VRX_OWNER`/`VRX_AGENT_SOCKET` in
   the environment and `VRX_AGENT_ARGS` verbatim. Please run it once P05 merges (or tell P05 which env names to honour).
