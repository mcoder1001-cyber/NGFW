#!/usr/bin/env bash
# Called by firstboot, never by package postinst. Preserve operator certificates.
set -euo pipefail
DIRECTORY=/etc/vrx/tls
if [[ $# == 2 && $1 == --directory ]]; then
  DIRECTORY=$(realpath -m -- "$2")
elif [[ $# != 0 ]]; then
  echo 'usage: tls-bootstrap.sh [--directory DIRECTORY]' >&2
  exit 2
fi
umask 077
install -d -m 0700 -- "$DIRECTORY"
exec 9>"$DIRECTORY/.bootstrap.lock"
flock -x 9
KEY=$DIRECTORY/server.key
CERT=$DIRECTORY/server.crt
if [[ -e $KEY || -e $CERT ]]; then
  [[ -f $KEY && ! -L $KEY && -f $CERT && ! -L $CERT ]] || { echo 'incomplete TLS pair; manual recovery required' >&2; exit 1; }
  # Validate the matching pair without printing private or public material.
  KEY_HASH=$(openssl pkey -in "$KEY" -pubout 2>/dev/null | openssl sha256)
  CERT_HASH=$(openssl x509 -in "$CERT" -pubkey -noout 2>/dev/null | openssl sha256)
  [[ $KEY_HASH == "$CERT_HASH" ]] || { echo 'TLS key/certificate mismatch' >&2; exit 1; }
  exit 0
fi
TEMP=$(mktemp -d "$DIRECTORY/.bootstrap.XXXXXXXX")
trap 'rm -rf -- "$TEMP"' EXIT
openssl req -x509 -newkey rsa:3072 -nodes -days 365 \
  -subj '/CN=VRX appliance' -addext 'subjectAltName=DNS:localhost,IP:127.0.0.1,IP:::1' \
  -keyout "$TEMP/server.key" -out "$TEMP/server.crt" >/dev/null 2>&1
chmod 0600 "$TEMP/server.key"
chmod 0644 "$TEMP/server.crt"
# Partial publication after a crash fails closed on the next run. Never silently
# replace an operator's certificate or private key to repair a partial pair.
mv -- "$TEMP/server.key" "$KEY"
mv -- "$TEMP/server.crt" "$CERT"
