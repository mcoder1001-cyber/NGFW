#!/usr/bin/env bash
# Bash entry point for the existing, API-compatible Ed25519 issuer (requires Node 22).
set -euo pipefail

# Editable defaults. Environment variables override these values.
KEY_DIR="${VRX_SIGNING_KEY_DIR:-${HOME}/.config/vrx/license-keys}"
PRIVATE_KEY="${VRX_SIGNING_PRIVATE_KEY:-${KEY_DIR}/vrx-license-signing.pem}"
PUBLIC_KEY="${VRX_SIGNING_PUBLIC_KEY:-${KEY_DIR}/vrx-license-public.pem}"
DEFAULT_DAYS="${VRX_LICENSE_DAYS:-365}"
DEFAULT_FEATURES="${VRX_LICENSE_FEATURES:-ipsec,wireguard,bgp,ospf,isis,ha}"
# Current built-in API key, for reference only: its private half was discarded.
# It cannot be used to issue licences. Configure the API with the generated public key.
BUILTIN_PUBLIC_KEY='-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEALR/JBpNQfFbGNAdIRgUMaDpPzrkPZkbavNmpQC7CP2o=
-----END PUBLIC KEY-----'

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
CLI="${SCRIPT_DIR}/vrx-license.mjs"

usage() {
  cat <<'HELP'
Usage:
  generate-license.sh init
  generate-license.sh api-env
  generate-license.sh builtin-public-key
  generate-license.sh issue --customer NAME --out FILE.vrxlic [issuer options]
  generate-license.sh verify FILE.vrxlic [--at ISO]
  generate-license.sh inspect FILE.vrxlic

init generates signing keys outside Git; existing keys are never overwritten.
api-env prints a shell export for VRX_LICENSE_PUBLIC_KEYS. Apply it to the API
process environment and restart the API before uploading a generated licence.
issue defaults to 365 days and all six licensed features, with unlimited limits.
Override with --key, --days, --expires, --features, --serial, --machine-id,
--machine-id-hash, --id, --not-before, or repeated --limit name=integer.
Keys and defaults can also be changed in the variables at the top of this file.
Requires Bash and Node 22; uses the existing vrx-license.mjs signing implementation.
HELP
}

command="${1:-help}"
if (($#)); then shift; fi
case "$command" in
  help|-h|--help) usage; exit 0 ;;
  builtin-public-key) printf '%s\n' "$BUILTIN_PUBLIC_KEY"; exit 0 ;;
esac
command -v node >/dev/null || { echo 'Node 22 is required.' >&2; exit 2; }

case "$command" in
  init)
    (($# == 0)) || { usage >&2; exit 2; }
    node "$CLI" keygen --out-dir "$KEY_DIR"
    printf '\nNext: %s api-env\n' "$0"
    ;;
  api-env)
    (($# == 0)) || { usage >&2; exit 2; }
    [[ -r "$PUBLIC_KEY" ]] || { echo "Public key missing: $PUBLIC_KEY (run init first)." >&2; exit 2; }
    # Bash %q quotes PEM safely, including newlines, for sourcing in a shell.
    printf 'export VRX_LICENSE_PUBLIC_KEYS=%q\n' "$(cat -- "$PUBLIC_KEY")"
    ;;
  issue)
    args=(--key "$PRIVATE_KEY" --features "$DEFAULT_FEATURES")
    has_expiry=false
    for arg in "$@"; do
      if [[ "$arg" == --days || "$arg" == --expires ]]; then has_expiry=true; fi
    done
    if [[ "$has_expiry" == false ]]; then args+=(--days "$DEFAULT_DAYS"); fi
    exec node "$CLI" issue "${args[@]}" "$@"
    ;;
  verify) exec node "$CLI" verify --pub "$PUBLIC_KEY" "$@" ;;
  inspect) exec node "$CLI" inspect "$@" ;;
  *) usage >&2; exit 2 ;;
esac
