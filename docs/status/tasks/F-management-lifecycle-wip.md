# Management TLS lifecycle WIP

Branch codex/management-lifecycle-parallel-20261003; base local7b57a284/tree of remotePR99. Dependency99 unmerged; draft candidate only. Owned mgmt-tls service/tests, user management doc, dedicated F-management-lifecycle-* docs. Code: sequential reload queue; late first-valid-certificate listener startup; enabled uses actual server.listening; shutdown gates queued startup. Tests added real HTTPS late start/actualstate and overlapping reload finalcertificate.
Current validation pending: schema dependency build, focused tests, ESLint and check gate running; no full quick to avoid excesshostload.
Remaining: certificate removal policy retains servingoldcert; state truthfulness/bindfailure retry targeted next. DB/browser/lab acceptance NOT RUN; freshreview missing.
Exact nextcommand: pnpm --filter @ngfw/api exec vitest run src/features/mgmt-tls/mgmt-tls.test.ts
Local/remoteSHA recorded by published checkpoint commit and manager report.
