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
  python3 - "$NGFW_INSTALL_ROOT" <<'PYROOT' || return 1
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
    local command resolved
    for command in apt-get curl gpg go npm corepack python3 tar; do
      resolved=$(command -v "$command") || return 1
      [[ $resolved == "$NGFW_INSTALL_STUB_DIR/$command" && -f $resolved && ! -L $resolved ]] || {
        echo "REFUSED: alternate-root command must be a regular stub: $command" >&2; return 1;
      }
    done
  elif [[ $NGFW_INSTALL_DRY_RUN == 0 && $EUID != 0 ]]; then
    echo 'run as root' >&2; return 1
  fi
}
ngfw_install_path() {
  local path=${1:?absolute target required} target
  [[ $path == /* && $path != / && $path != *'/../'* && $path != */.. ]] || return 1
  target=${NGFW_INSTALL_ROOT%/}$path
  python3 - "$target" <<'PYPATH' || return 1
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
