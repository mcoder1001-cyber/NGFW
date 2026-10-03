#!/usr/bin/env bash
# deploy/image/iso/tests/run.sh — unit-style tests of the P14 ISO pieces: no root, no network, no ISO, no mounts.
# Everything runs in a temp dir that is removed at the end. Exit 1 on any failed check.
#   bootstrap password writer · console banner (render / check-login with a fake database) · image setup (files,
#   unit symlinks, GRUB fragment) · storage configs = common/layout.tsv · user-data render + static autoinstall checks ·
#   GRUB menu render · pool index reproducibility + apt simulation parser · shellcheck (when installed)
# These subprocess scripts expand their own arguments; ok/bad always succeed.
# shellcheck disable=SC2016,SC2015
set -euo pipefail
export LC_ALL=C
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
ISO=$(cd "$HERE/.." && pwd -P)
C=$ISO/common
TMP=$(mktemp -d "${TMPDIR:-/tmp}/vrx-iso-tests.XXXXXX")
trap 'rm -rf -- "$TMP"' EXIT
PASS=0 FAIL=0
ok() { PASS=$((PASS + 1)); printf 'ok   %s\n' "$*"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$*"; }
check() { local d=$1; shift; if "$@" >/dev/null 2>&1; then ok "$d"; else bad "$d"; fi; }
mode() { stat -c %a "$1"; }

# ---------------------------------------------------------------- vrx-bootstrap-password
R=$TMP/r1; mkdir -p "$R"
out=$("$C/bin/vrx-bootstrap-password" --root "$R" 2>&1)
check "password: bootstrap.env written 0600" test "$(mode "$R/etc/vrx/bootstrap.env")" = 600
check "password: console copy written 0600, dir 0700" test "$(mode "$R/var/lib/vrx-image/console/bootstrap-password")$(mode "$R/var/lib/vrx-image/console")" = 600700
pw=$(sed -n 's/^VRX_BOOTSTRAP_ADMIN_PASSWORD=//p' "$R/etc/vrx/bootstrap.env")
check "password: 20 chars from the unambiguous alphabet" grep -qxE '[ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789]{20}' <<<"$pw"
check "password: user admin" grep -qx 'VRX_BOOTSTRAP_ADMIN_USER=admin' "$R/etc/vrx/bootstrap.env"
check "password: console copy holds the same password" test "$(sed -n 2p "$R/var/lib/vrx-image/console/bootstrap-password")" = "$pw"
check "password: never on stdout/stderr" bash -c '! grep -qF -- "$1" <<<"$2"' _ "$pw" "$out"
check "password: >= 12 chars (API PASSWORD_MIN)" test "${#pw}" -ge 12
rc=0; "$C/bin/vrx-bootstrap-password" --root "$R" >/dev/null 2>&1 || rc=$?
check "password: refuses to overwrite (exit 3)" test "$rc" = 3
check "password: existing file kept" test "$(sed -n 's/^VRX_BOOTSTRAP_ADMIN_PASSWORD=//p' "$R/etc/vrx/bootstrap.env")" = "$pw"
"$C/bin/vrx-bootstrap-password" --root "$R" --force >/dev/null 2>&1
pw2=$(sed -n 's/^VRX_BOOTSTRAP_ADMIN_PASSWORD=//p' "$R/etc/vrx/bootstrap.env")
check "password: --force makes a new random one" test "$pw2" != "$pw"
R2=$TMP/r2; mkdir -p "$R2/var/lib/vrx"; : > "$R2/var/lib/vrx/firstboot.done"; rc=0
"$C/bin/vrx-bootstrap-password" --root "$R2" >/dev/null 2>&1 || rc=$?
check "password: refuses on an initialised box" test "$rc" = 3
rc=0; "$C/bin/vrx-bootstrap-password" --root "$R2" --user 'Bad;Name' >/dev/null 2>&1 || rc=$?
check "password: refuses a bad user name" test "$rc" = 2
# Repeated runs produce distinct, correctly formed credentials.
all=''; for i in $(seq 1 16); do R3=$TMP/p$i; mkdir -p "$R3"; "$C/bin/vrx-bootstrap-password" --root "$R3" >/dev/null; all+=$(sed -n 2p "$R3/var/lib/vrx-image/console/bootstrap-password")$'\n'; rm -rf "$R3"; done
check "password: 16 runs, 16 distinct values" test "$(sort -u <<<"$all" | grep -c .)" = 16
check "password: all repeated values use the allowed alphabet" bash -c '! grep -vE "^([ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789]{20})?$" <<<"$1"' _ "$all"

# ---------------------------------------------------------------- vrx-console-banner
ISSUE=$R/etc/issue.d/50-vrx-console.issue
"$C/bin/vrx-console-banner" render --root "$R" > "$TMP/banner.out" 2>&1
check "banner: issue file 0600 while the password is shown" test "$(mode "$ISSUE")" = 600
check "banner: shows user and password" grep -qF "user admin  password $pw2" "$ISSUE"
check "banner: management address + URL as agetty escapes" grep -qF 'https://\4/' "$ISSUE"
check "banner: password not on stdout" bash -c '! grep -qF -- "$1" "$2"' _ "$pw2" "$TMP/banner.out"
check "banner: no stray backslash escapes besides \\n \\l \\4 \\6" bash -c '! grep -oE "\\\\." "$1" | grep -vxE "\\\\[nl46]"' _ "$ISSUE"
fakeq() { printf '#!/bin/sh\n%s\n' "$1" > "$TMP/q"; chmod +x "$TMP/q"; }
export VRX_CONSOLE_BANNER_TEST_QUERY=$TMP/q
fakeq 'echo 0'
"$C/bin/vrx-console-banner" check-login --root "$R" > /dev/null
check "check-login: kept before first boot is done" test -f "$R/var/lib/vrx-image/console/bootstrap-password"
mkdir -p "$R/var/lib/vrx"; : > "$R/var/lib/vrx/firstboot.done"
fakeq 'exit 2'
"$C/bin/vrx-console-banner" check-login --root "$R" > /dev/null
check "check-login: kept when the database is unreachable" test -f "$R/var/lib/vrx-image/console/bootstrap-password"
fakeq 'echo 1'
"$C/bin/vrx-console-banner" check-login --root "$R" > /dev/null
check "check-login: kept while the bootstrap admin is unused" test -f "$R/var/lib/vrx-image/console/bootstrap-password"
fakeq 'case "$1" in *"source = '"'"'bootstrap'"'"'"*"last_login is null"*"credential_gen = 0"*) echo 0 ;; *) echo 9 ;; esac'
"$C/bin/vrx-console-banner" check-login --root "$R" > /dev/null
check "check-login: deleted after the first login" test ! -e "$R/var/lib/vrx-image/console/bootstrap-password"
check "check-login: banner re-rendered without the password, 0644" test "$(mode "$ISSUE")" = 644
check "check-login: password gone from the issue" bash -c '! grep -qF -- "$1" "$2"' _ "$pw2" "$ISSUE"
check "check-login: URL still shown" grep -qF 'https://\4/' "$ISSUE"
unset VRX_CONSOLE_BANNER_TEST_QUERY

# ---------------------------------------------------------------- vrx-image-setup
S=$TMP/target; mkdir -p "$S"
"$C/bin/vrx-image-setup" --root "$S" --profile iso --serial-console > /dev/null
for f in usr/lib/vrx-image/vrx-bootstrap-password usr/lib/vrx-image/vrx-console-banner; do
  check "setup: $f 0755" test "$(mode "$S/$f")" = 755
done
for u in vrx-console-banner.service vrx-bootstrap-password.service; do
  check "setup: $u enabled (multi-user.target.wants)" test "$(readlink "$S/etc/systemd/system/multi-user.target.wants/$u")" = "/etc/systemd/system/$u"
done
check "setup: login timer enabled (timers.target.wants)" test -L "$S/etc/systemd/system/timers.target.wants/vrx-console-banner-login.timer"
check "setup: sysctl + modules files" test -f "$S/etc/sysctl.d/80-vrx-dataplane.conf" -a -f "$S/etc/modules-load.d/vrx-dataplane.conf"
check "setup: sysctl never lowers rmem_max (60-vrx-netlink.conf owns it)" bash -c '! grep -qE "^net\.core\.(r|w)mem_max" "$1"' _ "$S/etc/sysctl.d/80-vrx-dataplane.conf"
cmd=$(GRUB_CMDLINE_LINUX_DEFAULT="quiet" sh -c ". '$S/etc/default/grub.d/90-vrx-dataplane.cfg'; printf %s \"\$GRUB_CMDLINE_LINUX_DEFAULT\"")
check "setup: GRUB fragment appends to the existing default" test "${cmd%% *}" = quiet
for a in default_hugepagesz=2M hugepagesz=2M hugepages=1024 iommu=pt intel_iommu=on console=ttyS0,115200n8; do
  check "setup: kernel arg $a" grep -qw -- "$a" <<<"$cmd"
done
check "setup: no hardware-dependent isolcpus/1G pages" bash -c '! grep -qE "isolcpus|hugepagesz=1G" <<<"$1"' _ "$cmd"
cmd0=$(sh -c ". '$S/etc/default/grub.d/90-vrx-dataplane.cfg'; printf %s \"\$GRUB_CMDLINE_LINUX_DEFAULT\"")
check "setup: GRUB fragment with an empty default has no leading space" test "${cmd0:0:1}" != ' '
rc=0; "$C/bin/vrx-image-setup" --root / >/dev/null 2>&1 || rc=$?
check "setup: refuses --root /" test "$rc" != 0
"$C/bin/vrx-image-setup" --root "$S" --profile iso --serial-console > /dev/null
check "setup: idempotent (second run, same GRUB line)" test "$(GRUB_CMDLINE_LINUX_DEFAULT="quiet" sh -c ". '$S/etc/default/grub.d/90-vrx-dataplane.cfg'; printf %s \"\$GRUB_CMDLINE_LINUX_DEFAULT\"")" = "$cmd"

# ---------------------------------------------------------------- layout ↔ storage configs, user-data, grub
python3 - "$C/layout.tsv" "$ISO/autoinstall" > "$TMP/storage.txt" 2>&1 <<'PY' && ok "storage: plain + luks configs match layout.tsv (labels, sizes, fstype, mounts, luks, order)" || { bad "storage configs vs layout.tsv"; cat "$TMP/storage.txt"; }
import sys, yaml
rows = [l.split() for l in open(sys.argv[1]) if l.strip() and not l.startswith('#')]
labels = [r[0] for r in rows]
assert labels == ['VRX-EFI', 'vrx-rootA', 'vrx-rootB', 'vrx-log', 'vrx-pg', 'vrx-data'], labels   # wave-BC-numbers "P14"
for variant in ('plain', 'luks'):
    cfg = yaml.safe_load(open('%s/storage-%s.yaml' % (sys.argv[2], variant)))['storage']['config']
    by = {a['id']: a for a in cfg}
    parts = [a for a in cfg if a['type'] == 'partition']
    assert parts[0]['flag'] == 'bios_grub' and parts[0]['size'] == '1M' and 'partition_name' not in parts[0]
    assert [p['partition_name'] for p in parts[1:]] == labels
    assert [p['number'] for p in parts] == list(range(1, 8))
    disk = by['disk0']; assert disk['ptable'] == 'gpt' and disk['match'] == {'size': 'largest'}
    for (label, size, fs, mnt, role, luks), p in zip(rows, parts[1:]):
        assert str(p['size']) == ('-1' if size == 'rest' else size), (label, p['size'])
        fmt = [a for a in cfg if a['type'] == 'format' and a.get('label') == label]
        assert len(fmt) == 1, label; fmt = fmt[0]
        assert fmt['fstype'] == fs
        vol = by[fmt['volume']]
        if variant == 'luks' and luks == 'luks':
            assert vol['type'] == 'dm_crypt' and vol['volume'] == p['id'] and vol['keyfile'] == '/run/vrx-luks.key'
        else:
            assert vol is p, (variant, label)
        mounts = [a for a in cfg if a['type'] == 'mount' and a['device'] == fmt['id']]
        assert (mounts[0]['path'] if mounts else '-') == mnt, (label, mounts)
        assert bool(p.get('grub_device')) == (role == 'esp')
    paths = [a['path'] for a in cfg if a['type'] == 'mount']
    assert paths[0] == '/' and len(paths) == 5
print('ok')
PY

ud=$TMP/user-data
python3 "$ISO/lib/render.py" user-data "$ISO/autoinstall/user-data.in" "$ISO/autoinstall/storage-plain.yaml" "$ud" '0.1.0~dev+20260929.gabc' ubuntu-server-minimal
python3 - "$ud" "$ISO" > "$TMP/ud.txt" 2>&1 <<'PY' && ok "user-data: #cloud-config, autoinstall v1, no interactive section, offline apt, no identity/password, commands exist" || { bad "user-data static checks"; cat "$TMP/ud.txt"; }
import os, sys, yaml
text = open(sys.argv[1]).read(); iso = sys.argv[2]
assert text.startswith('#cloud-config\n')
d = yaml.safe_load(text); a = d['autoinstall']
assert list(d) == ['autoinstall'], list(d)
assert a['version'] == 1 and a['interactive-sections'] == []
assert 'identity' not in a and 'user-data' in a and a['user-data']['users'] == []        # no default user
assert a['apt']['fallback'] == 'offline-install' and a['apt']['geoip'] is False
assert a['refresh-installer'] == {'update': False}
assert a['ssh']['allow-pw'] is False and 'authorized-keys' not in a['ssh']
assert a['source']['id'] == 'ubuntu-server-minimal'
assert a['storage']['config'][0]['id'] == 'disk0'
for k in ('locale', 'keyboard', 'timezone', 'network', 'storage', 'late-commands', 'early-commands', 'shutdown'):
    assert k in a, k                                                                   # nothing left for a prompt
for cmd in a['early-commands'] + a['late-commands']:
    script = [x for x in cmd if x.startswith('/cdrom/vrx/')][0]
    assert os.path.isfile(os.path.join(iso, 'installer', os.path.basename(script))), script
assert 'password' not in a['user-data'] and 'identity' not in a
print('ok')
PY
python3 "$ISO/lib/render.py" user-data "$ISO/autoinstall/user-data.in" "$ISO/autoinstall/storage-luks.yaml" "$TMP/ud-luks" 1.0 ubuntu-server
check "user-data: luks variant renders and parses" python3 -c 'import sys, yaml; c = yaml.safe_load(open(sys.argv[1]))["autoinstall"]["storage"]["config"]; assert sum(a["type"] == "dm_crypt" for a in c) == 3' "$TMP/ud-luks"
rc=0; python3 "$ISO/lib/render.py" user-data "$ISO/autoinstall/user-data.in" "$ISO/autoinstall/storage-plain.yaml" "$TMP/x" '1.0;rm' ubuntu-server >/dev/null 2>&1 || rc=$?
check "user-data: odd version refused" test "$rc" != 0

cat > "$TMP/grub.cfg" <<'EOF'
set timeout=30

loadfont unicode

set menu_color_normal=white/black
menuentry "Try or Install Ubuntu Server" {
	set gfxpayload=keep
	linux	/casper/vmlinuz  ---
	initrd	/casper/initrd
}
menuentry "Ubuntu Server with the HWE kernel" {
	set gfxpayload=keep
	linux	/casper/hwe-vmlinuz  ---
	initrd	/casper/hwe-initrd
}
EOF
python3 "$ISO/lib/render.py" grub "$TMP/grub.cfg" "$TMP/grub.out" 1.2.3 ' console=tty0 console=ttyS0,115200n8'
check "grub: VRX install entry is the default (first, default=0, timeout 5)" bash -c 'grep -m1 menuentry "$1" | grep -q "Install VRX 1.2.3" && grep -qx "set default=0" "$1" && grep -qx "set timeout=5" "$1"' _ "$TMP/grub.out"
check "grub: autoinstall + nocloud seed on the kernel line" grep -qF 'linux	/casper/vmlinuz autoinstall ds=nocloud\;s=/cdrom/nocloud/ --- console=tty0 console=ttyS0,115200n8' "$TMP/grub.out"
check "grub: reinstall entry carries vrx.reinstall=1" grep -qF 'autoinstall vrx.reinstall=1 ds=nocloud' "$TMP/grub.out"
check "grub: original entries kept in a submenu" grep -qF 'submenu "Ubuntu Server installer (interactive, no VRX)"' "$TMP/grub.out"
check "grub: braces balanced" test "$(grep -o '{' "$TMP/grub.out" | wc -l)" = "$(grep -o '}' "$TMP/grub.out" | wc -l)"

# ---------------------------------------------------------------- pool helpers
export VRX_ISO_LOG_PREFIX=tests
# shellcheck source=../lib/common.sh
. "$ISO/lib/common.sh"
# shellcheck source=../lib/pool.sh
. "$ISO/lib/pool.sh"
cat > "$TMP/sim.txt" <<'EOF'
Reading package lists...
Inst libfoo1 (1.2-1 Ubuntu:26.04/resolute [amd64])
Inst vpp (26.06-release+vrx1 VRX appliance:resolute [amd64])
Inst libc6 [2.43-1] (2.43-2 Ubuntu:26.04/resolute-updates [amd64])
Conf vpp (26.06-release+vrx1 VRX appliance:resolute [amd64])
EOF
check "pool: Inst parser (new + upgrade lines, sorted)" test "$(pool_inst_lines "$TMP/sim.txt" | tr '\n' '|')" = 'libc6 2.43-2|libfoo1 1.2-1|vpp 26.06-release+vrx1|'
if command -v dpkg-deb >/dev/null && command -v dpkg-scanpackages >/dev/null; then
  mkdir -p "$TMP/debs"
  for p in vrx-meta libvppinfra zlib-x; do
    mkdir -p "$TMP/pk/$p/DEBIAN"
    printf 'Package: %s\nVersion: 1.0\nArchitecture: all\nMaintainer: t <t@t.invalid>\nDescription: t\n' "$p" > "$TMP/pk/$p/DEBIAN/control"
    dpkg-deb --root-owner-group -Zgzip -b "$TMP/pk/$p" "$TMP/debs/${p}_1.0_all.deb" >/dev/null
  done
  pool_repo "$TMP/debs" "$TMP/repo1" 1790000000; pool_repo "$TMP/debs" "$TMP/repo2" 1790000000
  check "pool: index + Release byte-identical across two runs (reproducible)" diff -r "$TMP/repo1/dists" "$TMP/repo2/dists"
  check "pool: Release date from the fixed epoch" grep -qx 'Date: Mon, 21 Sep 2026 14:13:20 UTC' "$TMP/repo1/dists/resolute/Release"
  check "pool: Release lists Packages sha256" grep -qE "^ $(sha256sum "$TMP/repo1/dists/resolute/main/binary-amd64/Packages" | cut -c1-64) [0-9]+ main/binary-amd64/Packages$" "$TMP/repo1/dists/resolute/Release"
  check "pool: Debian layout (pool/main/libv/libvppinfra)" test -f "$TMP/repo1/pool/main/libv/libvppinfra/libvppinfra_1.0_all.deb"
  check "pool: Packages has all three" test "$(grep -c '^Package:' "$TMP/repo1/dists/resolute/main/binary-amd64/Packages")" = 3
fi
export VRX_ISO_SCRATCH=$TMP/scratch; mkdir -p "$VRX_ISO_SCRATCH"
rc=0; ( vrx_guard_path /etc/vpp ) >/dev/null 2>&1 || rc=$?; check "guard: /etc/vpp refused" test "$rc" != 0
rc=0; ( vrx_guard_path /root/NGFW/x ) >/dev/null 2>&1 || rc=$?; check "guard: main checkout refused" test "$rc" != 0
rc=0; ( vrx_guard_path "$TMP/elsewhere" ) >/dev/null 2>&1 || rc=$?; check "guard: outside scratch refused" test "$rc" != 0
check "guard: inside scratch allowed" vrx_guard_path "$VRX_ISO_SCRATCH/work"
mkdir -p "$VRX_ISO_SCRATCH/foreign"; rc=0; ( vrx_rm_rf "$VRX_ISO_SCRATCH/foreign" ) >/dev/null 2>&1 || rc=$?
check "guard: rm refuses a dir without the marker" test "$rc" != 0 -a -d "$VRX_ISO_SCRATCH/foreign"

check "installer: failed inventory blocks destructive continuation" env PYTHONDONTWRITEBYTECODE=1 python3 "$HERE/test_disk_guard.py"

check "python: renderer regression cases" env PYTHONDONTWRITEBYTECODE=1 python3 "$HERE/test_render.py"
check "python: uncertain console credential cleanup recovery" env PYTHONDONTWRITEBYTECODE=1 python3 "$HERE/test_console_recovery.py"
check "python: VPP manifest completeness and archive parity" env PYTHONDONTWRITEBYTECODE=1 python3 "$HERE/test_vpp_manifest.py"

# ---------------------------------------------------------------- shellcheck
if command -v shellcheck >/dev/null; then
  check "shellcheck: all P14 shell code" shellcheck -x -P SCRIPTDIR "$ISO/build-iso.sh" "$ISO"/lib/*.sh "$ISO"/installer/*.sh "$C"/bin/* "$HERE/run.sh"
fi
check "python: render.py compiles" env PYTHONPYCACHEPREFIX="$TMP/pycache" python3 -m py_compile "$ISO/lib/render.py"

printf '\nTESTS: %d passed, %d failed\n' "$PASS" "$FAIL"
((FAIL == 0))
