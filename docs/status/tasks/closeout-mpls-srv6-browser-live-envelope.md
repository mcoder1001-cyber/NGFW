# Actual MPLS/SRv6 API and browser task envelope

Owner: mpls_srv6_resume; branch codex/mpls-srv6-browser-live; isolated worktree /root/ngfw-wt/codex-mpls-srv6-browser-live.
Owned paths: test/topology/mpls-srv6-browser-live/**; docs/status/tasks/closeout-mpls-srv6-browser-live*.
No production or shared runtime changes authorized by this task. Disposable slot14 native CLI requires different shared socket inode and isolated mount namespace. Private GlobalsOwner creates and removes its own table0. No license bypass.

Acceptance: actual API applied commit, exact native/readback state; missing encapSource HTTP400 pointer; actual native /128 deletion and API drift/recommit recovery; owned agent restart and unchanged native object output plus real resync logs; real browser login and configured MPLS interfaces/routes/tunnels/SR plus SRv6 SID/policy/steering/editor with native counters, English/Persian; baseline API revision rollback and strict absence; original management TLS/public leakage assertions and private cleanup.

Sources frozen mixed: HTTP-service64fffb9dda441f169834ecb1dfc06865a1c6d37d, web/agentff5333a98ce5688789d29b1149ddfc9febd3fd0f. Artifact pre/post hashes required. Whole latest appliance acceptance is not claimed. Attempt7 exact fixturec6d9c6a0 is active; failures1–6 retained. Reviewer independent_resume. Manager publishes each checkpoint and provides actual remote SHA; do not infer publication from local commit.

Recovery: inspect .scratch-mpls-srv6-attempt7.log and owned launcher session96563/process1049090 before launching another campaign. Finite900s child process group; slot14 flock. Next command after completion: preserve actual log/artifact manifests/screenshots, inspect cleanup, obtain independent final review. Product defects must first be reported to manager.
