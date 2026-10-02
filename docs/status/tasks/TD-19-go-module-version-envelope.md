# TD-19 Go module version derivation — bounded source

Own task/TD19-go-module-version-20261002 / NGFW-TD19-go-module-version,
base actualmain0098d93f114d0f6eed56fdc1ef82d2cdf753f793. Scope only script20,
owned TD19 fixture tests/docs. No currentPR86/main/board/other feature edits.

Observed scope gap: task requires GO_VER from go.mod, but script20 hardcodes
1.26.0. Canonical agent module contains go1.26. Derive one strict go directive
from repository path anchored to script directory, normalize major.minor to patch0,
then refuse unless derived version equals reviewed1.26.0 artifact pin. Keep official
SHA/platform/protoc/govpp/flags unchanged. Do not invent hashes for newer versions.
Reject malformed/duplicate/missing/nonregular module before mutation/network.
Fixture copies actual script into temporary repo structure, varies only temporary
module, fake mutation commands; current version and cwd independence meaningful.
TD19 dependency/source-only exception unchanged: not full taskDONE or labPASS.
No host package/daemon/network/download/execution. Independent review/gates needed.

Implemented: strict unique numeric directive AWK parser from canonical regular
non-symlink apps/agent/go.mod anchored to BASH_SOURCE script directory; explicit
1.26.0 and1.26 normalize to same existing official pin. Unsupported patch/minor/
major and malformed/duplicate/missing/read failure refuse before uname/root/APT/
network. No sourcing/eval or caller override module selection; old SHA/protoc/
govpp/install command flags unchanged.

Actual new fixture6PASS0.118s: copied real script into temporary repository,
normal and explicitpatch pins, changed versions, missing/duplicate/malformed and
trailingcontent directives, foreignCWD/environment, symlink and injected failed
AWK read. Read failure uses fake reader (chmod cannot represent unreadability for
root-run fixture); not claimed actual host permission acceptance. Every refused
case calls zero fake mutation commands; no host APT/network/install executed.
Existing explicit seven-suite runner36PASS8.522s zero errors/failures/skips/xfails.
Its initial run36FAILED1ERROR7 because sparsecheckout omitted deploy/vpp/VERSION/
verify.sh; after materializing unchanged deploy/vpp fixtures, genuine36PASS above.
New six-test file is separately executed, not silently added to original36 gate.
bash -n EXIT0; shellcheck unavailable locally (command not found, NOTPASS). Hosted
shellcheck/full gate still required. No claimed full installer/labTD19DONE.
Independent review pending; publication/merge are manager-owned next steps.
