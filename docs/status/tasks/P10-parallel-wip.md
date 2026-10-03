# P10 parallel resumed bundle work

Branch: codex/p10-resume-parallel-20261003. Base SHA: 19aa88a5.
Owned files: deploy/debian/bundle/**; docs/user/install/bundle.md;
docs/status/tasks/P10-parallel-*.
Local/remote checkpoint: initial checkpoint being published; resolve branch SHA
with git rev-parse HEAD and git ls-remote origin refs/heads/codex/p10-resume-parallel-20261003.

Completed: newer dpkg ar fixture fix rebuilds a valid archive with same control
metadata and changed payload; separate malformed truncated archive refusal.
Exporter reports whole serialized tar SHA-256 and archive_bytes after flush/fsync
from the saved file descriptor. Existing bytes retains payload sum. English and
Persian instructions explain separate trusted report and checking before extraction.

Validation in progress: test_verify.py, test_export.py and unchanged check gate.
Next commands: python3 deploy/debian/bundle/test_install.py;
tools/ci.sh --base origin/main. Real complete artifacts, signed repository,
checkout-free helpers, clean Ubuntu installation and hardware acceptance remain
NOT RUN/incomplete. No host installations or VPP/daemon changes performed.
