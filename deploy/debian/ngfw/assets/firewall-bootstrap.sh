#!/usr/bin/env bash
# Early firewall preparation only: no PostgreSQL, authentication or daemon startup.
set -euo pipefail
umask 077
[[ $EUID == 0 ]] || { echo 'firewall bootstrap requires root' >&2; exit 1; }
if [[ -e /etc/ngfw/bootstrap.env ]]; then
  [[ -f /etc/ngfw/bootstrap.env && ! -L /etc/ngfw/bootstrap.env && $(stat -c '%u:%a' /etc/ngfw/bootstrap.env) == 0:600 ]] || { echo 'invalid bootstrap environment metadata' >&2; exit 1; }
fi
[[ -n ${NGFW_BOOTSTRAP_MGMT_IF:-} ]] || { echo 'explicit management interface required' >&2; exit 1; }
install -d -m 0750 /etc/ngfw
install -d -m 0755 /etc/nftables.d
TEMP=$(mktemp /etc/nftables.d/.ngfw-base.XXXXXXXX)
trap 'rm -f -- "$TEMP"' EXIT
/usr/bin/python3 /usr/lib/ngfw/render-base-policy.py \
  --management-interface "$NGFW_BOOTSTRAP_MGMT_IF" \
  --punt-interfaces "${NGFW_BOOTSTRAP_PUNT_IFS:-}" > "$TEMP"
[[ -d /sys/class/net/$NGFW_BOOTSTRAP_MGMT_IF ]] || { echo 'management interface does not exist' >&2; exit 1; }
# All interface strings were validated by the pure renderer, no command expansion.
IFS=, read -r -a PUNTS <<< "${NGFW_BOOTSTRAP_PUNT_IFS:-}"
for interface in "${PUNTS[@]}"; do
  [[ -z $interface || -d /sys/class/net/$interface ]] || { echo 'punt interface does not exist' >&2; exit 1; }
done
/usr/sbin/nft -c -f "$TEMP"
chmod 0644 "$TEMP"
sync -f "$TEMP"
mv "$TEMP" /etc/nftables.d/ngfw-base.nft
# Persistent interface inputs survive deletion of the secret-bearing bootstrap file.
TEMP=$(mktemp /etc/ngfw/.base-policy-env.XXXXXXXX)
printf 'NGFW_BOOTSTRAP_MGMT_IF=%s\nNGFW_BOOTSTRAP_PUNT_IFS=%s\n' "$NGFW_BOOTSTRAP_MGMT_IF" "${NGFW_BOOTSTRAP_PUNT_IFS:-}" > "$TEMP"
chmod 0600 "$TEMP"
sync -f "$TEMP"
mv "$TEMP" /etc/ngfw/base-policy.env
sync -f /etc/ngfw
