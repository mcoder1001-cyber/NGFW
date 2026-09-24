#!/usr/bin/env bash
# Add the third-party APT repositories VRX needs. Run once, as root, before any install script.
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }
CODENAME="$(. /etc/os-release && echo "$VERSION_CODENAME")"
[[ "$CODENAME" == "resolute" ]] || echo "WARNING: tested on Ubuntu 26.04 (resolute); found '$CODENAME'"

apt-get update
apt-get install -y curl gnupg ca-certificates lsb-release apt-transport-https

# --- FD.io VPP -------------------------------------------------------------
# 'release' tracks the latest stable VPP. Pin a specific train instead (e.g. 2606)
# once you choose the baseline; a data plane must never float.
VPP_REPO="${VPP_REPO:-release}"
curl -s "https://packagecloud.io/install/repositories/fdio/${VPP_REPO}/script.deb.sh" | bash

# --- FRRouting -------------------------------------------------------------
# resolute is a new LTS (Apr 2026); if deb.frrouting.org hasn't published a
# 'resolute' suite yet, the apt-get update below 404s on this repo - fall back
# to Ubuntu's own 'frr' package (present in the resolute archive) until it does.
curl -s https://deb.frrouting.org/frr/keys.gpg | tee /usr/share/keyrings/frrouting.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/frrouting.gpg] https://deb.frrouting.org/frr ${CODENAME} frr-stable" \
  > /etc/apt/sources.list.d/frr.list

# --- Node.js 22 LTS --------------------------------------------------------
curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
  | gpg --dearmor -o /usr/share/keyrings/nodesource.gpg
echo "deb [signed-by=/usr/share/keyrings/nodesource.gpg] https://deb.nodesource.com/node_22.x nodistro main" \
  > /etc/apt/sources.list.d/nodesource.list

apt-get update
echo "repositories added: fdio/${VPP_REPO}, frr-stable, nodesource node_22.x"
echo "NOTE: Kea and Valkey need no extra repo on resolute - kea-dhcp4-server (3.0.x)"
echo "      and valkey-server are already in the Ubuntu archive. See docs/09-os-packages.md."
