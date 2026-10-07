#!/usr/bin/env bash
# Root-mode refers exclusively to the owned private outer network namespace.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "$HERE/private-fib.py" "$@"
