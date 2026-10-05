#!/bin/sh
# Native route-based production-agent acceptance; strongSwan is a peer only.
set -eu
exec python3 "$(dirname "$0")/host-acceptance.py" "$@"
