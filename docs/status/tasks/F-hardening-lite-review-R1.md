# F-hardening-lite: independent R1 review

Frozen final source 726d9214 (remote4707bffb8c84a25b5fd332b1bd095866512ccf43). Reviewer owns reports only.

Source inspected: canonical offline-root-only stage; all destination symlink preflight before mutation; explicit opt-in SSH management profile; repeat-stage retains selected controls; actual management file drift detection; exact readonly compliance report and explicit exceptions; signed Release immutable snapshot authentication followed by authenticated member digest checks. Systemd profiles retain daemon write directories/address families/capabilities, omit VPP tightening and Node JIT incompatible hardening. Packaging ships profiles under /usr/lib without activating shared host controls. Runtime compatibility and real signed APT install are lab acceptance, not claimed by fixture tests.

The final Go module under test/topology/hardening-lite invokes the Python suite in unchanged quick CI. Ten regression tests cover baseline drift, root/symlink refusal before writes, opt-in key validation/management names, repeated selection, undeclared profiles, compatibility policy, signed/tampered/missing/unknown key/traversal, metadata replacement after gpgv and real old/new/overlap/retirement trust. Canonical task report now present; earlier missing canonical report/path clarification resolved.

Verdict: APPROVE. No correctness finding identified on final source. Independent unchanged complete quick gate PASS36m12s, all checks including generation clean; see T1 report for exact command and limits.
