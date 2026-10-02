# P10 dedicated hosted packaging fixture gate

Branch task/P10-packaging-ci-20261002; isolated worktree NGFW-packaging-ci.
Base local 360bbde9 (PR67 source); frozen PR67/product files unchanged.
Owned: dedicated workflow, its Python gate wrapper, task evidence and targeted
central acceptance annotation only. tools/ci.sh/quick stays unchanged.

Audited all six fixture files before enabling hosted execution. Actual package
creation uses dpkg-deb on temporary directories, not installation. Runtime and
firstboot fixtures redirect host paths and stub APT/systemctl/DB/nginx/startup
commands. TLS is real OpenSSL against temporary files. Signing uses real GPG
with isolated ephemeral keys and gpgv verification; VPP verification and reprepro
are fixture commands, so this is not artifact provenance or real publication.
No production daemon start, nft load, service installation or external release
publication occurs. Root/metadata fixture adaptations remain source control-flow
checks, not appliance privilege acceptance.

Workflow runs Ubuntu24.04 source fixtures with pinned checkout/setup-node actions
from existing CI and explicit Node22.23.2, read-only contents, no saved credentials
and no package installation. Required existing tool absence fails preflight.
All discovered tests run verbosely; ANY skip, error, failure or zero tests fails
the dedicated gate. Local GPG agent sockets remain unavailable: one signing test
is expected to SKIP locally and wrapper must fail, never be reported PASS.
Hosted signing is NOT RUN until a successful actual workflow run establishes it.
This independent gate supplements the unchanged mandatory full quick gate;
it does not certify Ubuntu26 appliance installation, boot or traffic acceptance.

Actual checkpoint validation at 1cd9ae51: 26 fixture tests in 6.523 seconds,
25 PASS / 1 signing SKIP, gate process exit1 as required. YAML parsed and git
whitespace check passed. Pending independent R7 review and actual hosted run.
Frozen PR67 final independent integration approval copied unchanged into this
new branch; no PR67 tree mutation. Next command for reproduction:
`python3 .github/scripts/packaging-fixtures.py` (local signing socket limitation
means expected exit1, not a complete gate pass).

Independent R7/R1 found that unittest expected failures can still make
wasSuccessful() true. Corrected gate rejects expectedFailures explicitly and
summarizes expected failures and unexpected successes. Seven separate tiny real
unittest suites now execute the gate policy: success exits0; failure, error,
skip, expected failure, unexpected success and zero tests each exit1. All seven
policy regressions PASS (0.004s); the product fixture suite was not run twice.
Workflow executes policy tests before all product fixtures. Bytecode cache files
accidentally staged by the first guard-test run were immediately removed from
the next checkpoint and are ignored; final source tree contains no bytecode.
Fresh independent review of this correction remains pending. Real hosted signing
still NOT RUN; local product signing skip remains an honest failing gate result.
