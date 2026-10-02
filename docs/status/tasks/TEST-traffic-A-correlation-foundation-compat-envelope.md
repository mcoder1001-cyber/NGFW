# Correlation integration: preserve the original foundation gate

Own branch `task/traffic-correlation-foundation-compat-20261002`, isolated
`NGFW-correlation-foundation-compat`. Base is independently reviewed correlation
CI `9b2b1e8dc414bb5a881e3087479341d16d732f3d`; traffic source remains identical
approved1427. Producer branch is frozen and untouched.

Owned: foundation fixture runner/workflow compatibility only, this envelope/WIP.
The approved foundation workflow is copied verbatim from final
`46f3a42fac6784f021d89506041fe6b5a153b9b3` then labelled original16 and extended
with a strict named-contract policy step. Its ca486 workflow identity was checked.
No other workflow, broad quick gate, source/test/assertion, board or arbitration
row is changed; all base review metadata and rows are retained.

Original16 exact method names are extracted from fda0ddc7 test_foundation and
 test_evidence AST (nine Foundation, seven Evidence). Every name remains present
in reviewed correlation source. Explicit loading preserves all original cases,
requires unique16 loaded/completed and rejects missing methods and all non-success
outcomes. This does not discard expanded coverage: mandatory correlation33 runs
ALL current Foundation10+Evidence12+Correlation11 cases; later producer40 adds
ALL Producer7. An expanded source composition cannot merge using original16 alone.

New compatibility code needs fresh independent R1/R2/R7 review. Manager composes
onto actual main after foundation lands, preserves both gate workflows and prior
rulings, makes the final single commit, runs unchanged complete quick plus actual
original16 AND whole correlation33 on exact published tree before expected-head
merge, then verifies main CI. Producer40 follows in its own approved integration.
No stale green, lab waiver or inactive source fixture becomes traffic acceptance.
