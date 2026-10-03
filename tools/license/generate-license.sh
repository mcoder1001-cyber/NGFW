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
  generate-license.sh                         # Generate a full-feature licence
  generate-license.sh all [CUSTOMER [FILE]]    # Same, with optional name/path
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
The easy mode creates keys if missing, verifies the licence, and writes api.env
next to it. All six features are enabled with no limits, valid for DEFAULT_DAYS.
HELP
}

command="${1:-all}"
if (($#)); then shift; fi
case "$command" in
  help|-h|--help) usage; exit 0 ;;
  builtin-public-key) printf '%s\n' "$BUILTIN_PUBLIC_KEY"; exit 0 ;;
esac
command -v node >/dev/null || { echo 'Node 22 is required.' >&2; exit 2; }

case "$command" in
  all)
    (($# <= 2)) || { usage >&2; exit 2; }
    customer="${1:-${VRX_LICENSE_CUSTOMER:-Local administrator}}"
    license_file="${2:-${VRX_LICENSE_OUTPUT:-license.vrxlic}}"
    env_file="${license_file}.api.env"
    [[ ! -e "$license_file" && ! -e "$env_file" ]] || {
      echo 'Output already exists. Choose a new path: all "Customer" new.vrxlic' >&2
      exit 2
    }
    if [[ ! -e "$PRIVATE_KEY" && ! -e "$PUBLIC_KEY" ]]; then
      # keygen writes these fixed filenames; custom paths must be supplied as a pair.
      [[ "$PRIVATE_KEY" == "$KEY_DIR/vrx-license-signing.pem" && "$PUBLIC_KEY" == "$KEY_DIR/vrx-license-public.pem" ]] || {
        echo 'Custom key paths require an existing private/public key pair.' >&2; exit 2;
      }
      node "$CLI" keygen --out-dir "$KEY_DIR"
    fi
    [[ -r "$PRIVATE_KEY" && -r "$PUBLIC_KEY" ]] || {
      echo 'Both signing keys must exist and be readable. Check the key paths.' >&2; exit 2;
    }
    # Verify the pair before issuing; a mismatched public key would fail API acceptance.
    node --input-type=module - "$PRIVATE_KEY" "$PUBLIC_KEY" <<'JS'
import { readFileSync } from 'node:fs';
import { createPrivateKey, createPublicKey } from 'node:crypto';
const privateKey = createPrivateKey(readFileSync(process.argv[2]));
const publicKey = createPublicKey(readFileSync(process.argv[3]));
if (privateKey.asymmetricKeyType !== 'ed25519' ||
    !createPublicKey(privateKey).export({ type: 'spki', format: 'der' })
      .equals(publicKey.export({ type: 'spki', format: 'der' }))) {
  console.error('The Ed25519 signing/public keys do not match.');
  process.exit(2);
}
JS
    node "$CLI" issue --key "$PRIVATE_KEY" --customer "$customer" --days "$DEFAULT_DAYS" \
      --features ipsec,wireguard,bgp,ospf,isis,ha --out "$license_file"
    node "$CLI" verify --pub "$PUBLIC_KEY" "$license_file"
    # JSON quoting gives both shell and systemd a single value with literal \n escapes;
    # parsePublicKeys in the API expands these escapes back into PEM newlines.
    node --input-type=module - "$PUBLIC_KEY" "$env_file" <<'JS'
import { readFileSync, writeFileSync } from 'node:fs';
const pem = readFileSync(process.argv[2], 'utf8').trim().replace(/\r?\n/g, '\\n');
writeFileSync(process.argv[3], `VRX_LICENSE_PUBLIC_KEYS=${JSON.stringify(pem)}\n`, { flag: 'wx', mode: 0o644 });
JS
    printf '\nلایسنس همه قابلیت‌ها ساخته شد: %s\nاعتبار: %s روز؛ بدون محدودیت تعداد.\n' "$license_file" "$DEFAULT_DAYS"
    printf 'برای پذیرش در سیستم، خط فایل %s را در /etc/vrx/api.env قرار بده و vrx-api را ری‌استارت کن.\n' "$env_file"
    printf 'سپس فایل لایسنس را در System → Licence بارگذاری کن.\n'
    ;;
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
