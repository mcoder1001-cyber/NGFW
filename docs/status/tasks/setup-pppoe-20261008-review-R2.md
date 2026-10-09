# Setup PPPoE independent R2 review

Verdict: **APPROVE**, scoped to setup wizard source changes at local `6ecc64693e46eeb9d39ff4e5d0772e1b2c982f57`, tree `282040eb8853afe0eabbecdcd2fbb951d7366739`, published `cf8cddf8eb80bf6e1a45022483462d5a7877905b`. Reviewer: audit_merged, 2026-10-08.

Reviewed schema/builder, API preview/stage and datastore transaction boundary, React form, en/fa strings and focused tests. The ISP credential input is username plus an existing password reference; it cannot contain an inline ISP password. Explicit audit summaries omit credentials. The physical parent and distinct logical setup-pppoe WAN have separate roles; NAT and WAN ACL use the logical interface. Incompatible parent use and unowned or modified logical-name collisions are refused. Removing a prior wizard logical WAN refuses remaining unrelated references. The existing setup-wide NAT policy replacement remains unchanged by this delta.

Candidate staging retains transactional expected-revision and empty-candidate checks, current administrator password validation, and the normal validate/confirmed-commit flow. Preview does not edit candidate. Unrelated objects outside the existing wizard policy/addressing scope remain cloned and preserved.

One review blocker was corrected before approval: live collision detection initially searched for setup-pppoe in a response requested only for the physical WAN. The final request explicitly includes setup-pppoe, and the regression mock filters by requested names, matching the actual agent RPC. Thus an existing live logical-name collision is refused before staging.

Independent focused execution with Node 22.23.2: schema setup suite **12 PASS**; API setup controller suite **16 PASS**; web setup and regression suites **11 PASS**, including English/Persian reference input and clearing, full PPP staging/confirmation and candidate-preservation cases. No CI or native networking actions were run. Actual ISP dialing and packet/anti-lockout acceptance remain laboratory work; this approval does not substitute for the separate carrier runtime/security review or the final combined gate.
