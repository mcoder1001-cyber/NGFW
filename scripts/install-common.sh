#!/usr/bin/env bash
# Shared installer seam. Alternate roots are fixture-only: privileged package
# managers do not become safe merely because their output paths were prefixed.
ngfw_install_init() {
  NGFW_INSTALL_DRY_RUN=0
  if [[ ${1:-} == --dry-run && $# == 1 ]]; then
    NGFW_INSTALL_DRY_RUN=1
  elif [[ $# != 0 ]]; then
    echo 'usage: installer [--dry-run]' >&2; return 2
  fi
  NGFW_INSTALL_ROOT=${NGFW_INSTALL_ROOT:-/}
  /usr/bin/python3 - "$NGFW_INSTALL_ROOT" <<'PYROOT' || return 1
import pathlib, sys
value = sys.argv[1]
p = pathlib.Path(value)
if value.startswith('//') or not p.is_absolute() or '..' in p.parts or str(p) != value:
    raise SystemExit('REFUSED: install root must be a normalized absolute directory')
for ancestor in [p, *p.parents]:
    if ancestor.is_symlink():
        raise SystemExit('REFUSED: install root cannot traverse symlinks')
if value != '/' and any(child.is_symlink() for child in p.rglob('*')):
    raise SystemExit('REFUSED: fixture install root cannot contain symlinks')
if not p.is_dir():
    raise SystemExit('REFUSED: install root must already exist')
PYROOT
  if [[ $NGFW_INSTALL_ROOT != / && $NGFW_INSTALL_DRY_RUN == 0 ]]; then
    [[ ${NGFW_INSTALL_STUBS:-} == 1 && -n ${NGFW_INSTALL_STUB_DIR:-} ]] || {
      echo 'REFUSED: alternate-root execution requires explicit recording stubs' >&2; return 1;
    }
    local recorder_dir
    recorder_dir=$(cd -- "$(/usr/bin/dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
    /usr/bin/python3 - "$NGFW_INSTALL_STUB_DIR" "$recorder_dir/install-recording-stub.py" "$NGFW_INSTALL_ROOT" <<'PYSTUBS' || return 1
import pathlib, sys
stubs, contract, root = map(pathlib.Path, sys.argv[1:])
if not stubs.is_absolute() or stubs.is_symlink() or not stubs.is_dir():
    raise SystemExit('REFUSED: trusted recording harness directory required')
expected = contract.read_bytes()
required = {'apt-get', 'curl', 'gpg', 'go', 'npm', 'corepack', 'python3', 'tar', 'pip'}
for path in [*(stubs / name for name in required), *stubs.iterdir()]:
    if not path.is_file() or path.is_symlink() or path.name not in required or path.read_bytes() != expected:
        raise SystemExit('REFUSED: command differs from shipped recording harness: ' + path.name)
# An existing rooted executable never gains authority through a PATH prepend.
for path in root.rglob('*'):
    if path.is_file() and path.stat().st_mode & 0o111 and path.read_bytes() != expected:
        raise SystemExit('REFUSED: unchecked rooted fixture executable: ' + str(path))
PYSTUBS
    PATH="$NGFW_INSTALL_STUB_DIR:/usr/bin:/bin"
    export PATH

  elif [[ $NGFW_INSTALL_DRY_RUN == 0 && $EUID != 0 ]]; then
    echo 'run as root' >&2; return 1
  fi
}
ngfw_install_path() {
  local path=${1:?absolute target required} target
  [[ $path == /* && $path != / && $path != *'/../'* && $path != */.. ]] || return 1
  target=${NGFW_INSTALL_ROOT%/}$path
  /usr/bin/python3 - "$target" <<'PYPATH' || return 1
import pathlib, sys
p = pathlib.Path(sys.argv[1])
for ancestor in [p, *p.parents]:
    if ancestor.is_symlink():
        raise SystemExit('REFUSED: installation path cannot traverse symlinks')
PYPATH
  printf '%s\n' "$target"
}
ngfw_install_plan() {
  printf 'DRY-RUN root=%q: %s\n' "$NGFW_INSTALL_ROOT" "$1"
}
