#!/usr/bin/env bash
# Appliance firstboot only. Never run this script on the shared development host.
set -euo pipefail
umask 077
[[ $EUID == 0 ]] || { echo 'firstboot requires root' >&2; exit 1; }
exec 9>/run/lock/ngfw-firstboot.lock
flock -x 9
if [[ -e /var/lib/ngfw/firstboot-complete ]]; then
  [[ ! -e /etc/ngfw/bootstrap.env ]] || rm -- /etc/ngfw/bootstrap.env
  exit 0
fi
[[ -f /etc/ngfw/bootstrap.env && ! -L /etc/ngfw/bootstrap.env ]] || { echo 'missing bootstrap.env' >&2; exit 1; }
[[ $(stat -c '%u:%a' /etc/ngfw/bootstrap.env) == 0:600 ]] || { echo 'bootstrap.env must be root-owned 0600' >&2; exit 1; }
[[ -n ${NGFW_BOOTSTRAP_ADMIN_USER:-} && -n ${NGFW_BOOTSTRAP_ADMIN_PASSWORD:-} ]] || { echo 'missing bootstrap administrator' >&2; exit 1; }
# Fixed SQL identifiers; no bootstrap values enter a shell or SQL command.
runuser -u postgres -- psql -X -v ON_ERROR_STOP=1 -d postgres >/dev/null <<'SQL'
SELECT 'CREATE ROLE ngfw LOGIN' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname='ngfw')
\gexec
SQL
runuser -u postgres -- psql -X -v ON_ERROR_STOP=1 -d postgres >/dev/null <<'SQL'
SELECT 'CREATE DATABASE ngfw OWNER ngfw' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname='ngfw')
\gexec
SQL
install -d -m 0750 -o root -g ngfw /var/lib/ngfw /etc/ngfw
if [[ ! -e /var/lib/ngfw/secret.key ]]; then
  TEMP=$(mktemp /var/lib/ngfw/.secret.XXXXXXXX)
  openssl rand 32 > "$TEMP"
  chmod 0600 "$TEMP"
  chown ngfw:ngfw "$TEMP"
  mv "$TEMP" /var/lib/ngfw/secret.key
fi
[[ -f /var/lib/ngfw/secret.key && ! -L /var/lib/ngfw/secret.key ]] || { echo 'invalid secret key file' >&2; exit 1; }
[[ $(stat -c '%s:%a:%U' /var/lib/ngfw/secret.key) == 32:600:ngfw ]] || { echo 'invalid secret key metadata' >&2; exit 1; }
if [[ ! -e /etc/ngfw/api.env ]]; then
  TEMP=$(mktemp /etc/ngfw/.api-env.XXXXXXXX)
  printf '%s\n' 'NGFW_DATABASE_URL=postgresql:///ngfw?host=/var/run/postgresql&user=ngfw' 'NGFW_SECRET_KEY_FILE=/var/lib/ngfw/secret.key' > "$TEMP"
  JWT_SECRET=$(openssl rand -hex 32)
  [[ $JWT_SECRET =~ ^[a-f0-9]{64}$ ]] || { echo 'JWT key generation failed' >&2; exit 1; }
  printf 'NGFW_JWT_SECRET=%s\n' "$JWT_SECRET" >> "$TEMP"
  chmod 0600 "$TEMP"
  mv "$TEMP" /etc/ngfw/api.env
fi
[[ -f /etc/ngfw/api.env && ! -L /etc/ngfw/api.env && $(stat -c '%u:%a' /etc/ngfw/api.env) == 0:600 ]] || { echo 'invalid API environment file' >&2; exit 1; }
# Initial API environment is canonical: optional runtime overrides are configured
# after successful provisioning. In particular JWT_KEY_FILE wins over JWT_SECRET.
/usr/bin/python3 - /etc/ngfw/api.env <<'PYENV'
import pathlib, sys
allowed = {'NGFW_DATABASE_URL', 'NGFW_SECRET_KEY_FILE', 'NGFW_JWT_SECRET'}
lines = pathlib.Path(sys.argv[1]).read_text(encoding='utf-8').splitlines()
if any(line.split('=', 1)[0] not in allowed for line in lines):
    sys.exit('initial API environment contains unsupported overrides')
PYENV
[[ -z ${NGFW_JWT_KEY_FILE:-} ]] || { echo 'bootstrap JWT key-file override is unsupported' >&2; exit 1; }
for VARIABLE in NGFW_DATABASE_URL NGFW_SECRET_KEY_FILE; do
  COUNT=$(grep -Ec "^[[:space:]]*${VARIABLE}[[:space:]]*=" /etc/ngfw/api.env || true)
  [[ $COUNT == 1 ]] || { echo 'invalid persisted API environment assignment count' >&2; exit 1; }
done
grep -Fxq 'NGFW_DATABASE_URL=postgresql:///ngfw?host=/var/run/postgresql&user=ngfw' /etc/ngfw/api.env || { echo 'API database configuration requires manual review' >&2; exit 1; }
grep -Fxq 'NGFW_SECRET_KEY_FILE=/var/lib/ngfw/secret.key' /etc/ngfw/api.env || { echo 'API secret configuration requires manual review' >&2; exit 1; }
JWT_COUNT=$(grep -Ec '^[[:space:]]*NGFW_JWT_SECRET[[:space:]]*=' /etc/ngfw/api.env || true)
[[ $JWT_COUNT == 1 ]] || { echo 'invalid persisted JWT assignment count' >&2; exit 1; }
JWT_SECRET=$(sed -n 's/^NGFW_JWT_SECRET=//p' /etc/ngfw/api.env)
[[ $JWT_SECRET =~ ^[a-f0-9]{64}$ ]] || { echo 'invalid or duplicate persisted JWT key' >&2; exit 1; }
export NGFW_JWT_SECRET=$JWT_SECRET
# Parse generated environment as data instead of sourcing shell code.
NGFW_DATABASE_URL=$(sed -n 's/^NGFW_DATABASE_URL=//p' /etc/ngfw/api.env)
NGFW_SECRET_KEY_FILE=$(sed -n 's/^NGFW_SECRET_KEY_FILE=//p' /etc/ngfw/api.env)
export NGFW_DATABASE_URL NGFW_SECRET_KEY_FILE NODE_ENV=production
(cd /usr/lib/ngfw/api && runuser --preserve-environment -u ngfw -- /usr/bin/node bootstrap-db.mjs)
/usr/lib/ngfw/tls-bootstrap.sh
# The appliance base policy needs LCP before the first configuration commit.
# An empty device list still renders no-pci and protects management NICs.
install -m 0600 /usr/lib/ngfw/initial-dataplane.json /var/lib/ngfw/initial-dataplane.json
/usr/lib/ngfw/bin/ngfw-startupgen -o /etc/vpp/startup.conf /var/lib/ngfw/initial-dataplane.json
# Later startup changes use the separately gated apply-startup.sh, never this tool.
if [[ -e /etc/nginx/sites-enabled/ngfw || -L /etc/nginx/sites-enabled/ngfw ]]; then
  [[ $(readlink /etc/nginx/sites-enabled/ngfw) == /etc/ngfw/nginx.conf ]] || { echo 'foreign nginx site; manual review required' >&2; exit 1; }
else
  ln -s /etc/ngfw/nginx.conf /etc/nginx/sites-enabled/ngfw
fi
if [[ -L /etc/nginx/sites-enabled/default && $(readlink /etc/nginx/sites-enabled/default) == /etc/nginx/sites-available/default ]]; then
  rm -- /etc/nginx/sites-enabled/default
fi
/usr/sbin/nginx -t
# Delete bootstrap credentials only after migrations, seed confirmation, TLS,
# startup rendering and nginx validation succeeded. Publish durable completion
# before credential deletion so a crash between the operations safely resumes cleanup.
TEMP=$(mktemp /var/lib/ngfw/.firstboot-complete.XXXXXXXX)
printf 'completed\n' > "$TEMP"
sync -f /var/lib/ngfw/secret.key
sync -f /etc/ngfw/api.env
sync -f "$TEMP"
mv "$TEMP" /var/lib/ngfw/firstboot-complete
sync -f /var/lib/ngfw
sync -f /etc/ngfw
rm -- /etc/ngfw/bootstrap.env
systemctl disable ngfw-firstboot.service
