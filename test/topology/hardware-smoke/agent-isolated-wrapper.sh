#!/bin/sh
# Adapt legacy fixtures that strip slot ID configuration, only on our disposable VPP.
set -eu
# Some fixtures erase environment markers. The bound private startup file survives.
[ -f /run/vpp/startup.conf ] && grep -q '^api-segment { prefix fulltest[0-9][0-9]* }$' /run/vpp/startup.conf || {
    echo 'requires the private VPP mount from isolated-vpp.py' >&2
    exit 2
}
case "${VRX_OWNER:-}" in
    w[0-9]*) ;;
    *) echo 'requires an explicit test slot owner' >&2; exit 2 ;;
esac
[ "${VRX_GLOBALS_OWNER:-}" = 0 ] || { echo 'requires globals ownership off' >&2; exit 2; }
[ -n "${VRX_AGENT_STATE_DIR:-}" ] && [ -n "${VRX_AGENT_SOCKET:-}" ] || {
    echo 'requires explicit test state and socket paths' >&2; exit 2
}
export VRX_VPP_ID_RANGE=all
exec "$(dirname "$0")/../../../apps/agent/bin/vrx-agent" "$@"
