# TD-19 official upstream signer authority follow-up

Docs-only envelope/checkpoint. Branch task/TD19-frr-upstream-authority-20261002,
isolated NGFW-TD19-frr-upstream-authority, exact base
78218e6da8b623debf2542bebbe15af2fcc5a809. Own only this report.
Earlier b0f12494/8c639171 research covered homepage, downloaded public bundle,
metadata signer and Packages digest. That established raw signature consistency,
not independent authority for A90FC36D9429409798E9C2D874DEED43AB194DBF.
The canonical certificate transformation fixes mechanics, not signer authority.

New bounded read-only scope: inspect official FRRouting repositories, packaging/
build/publish source, release documents and attributable commits for separately
published full fingerprint or trust rotation. No source/pin/gate/default trust,
PR90/main/board or host installation changes; no keyserver trust substitution.
Negative evidence must name actual inspected paths, not claim all upstream
history searched. NodeSource Ubuntu26.04 support remains unresolved.

Initial search: official frr README and releases link deb.frrouting.org; no full
A90 fingerprint found by exact indexed official-domain search. This is not an
exhaustive repository search. Research continuing, not authority resolution.
Remote publication not claimed; local checkpoint sent to parent manager.
