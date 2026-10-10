#!/bin/bash
# --preflight touches only the owned RAM probe; --repair requires phase release.
set -eu
export LC_ALL=C PATH=/usr/sbin:/usr/bin:/sbin:/bin
umask 077
case ${1:-} in --preflight|--repair) ;; *) exit 2;; esac
undo=/root/recovery-private/root-repair.undo
probe=/root/recovery-private/undo-write-preflight.test
[[ $(stat -Lc %d /) == 46 && $(stat -Lc %d /root/recovery-private) == 46 ]]
[[ $(stat -Lc %a /root/recovery-private) == 700 ]]
[[ ! -e $undo && ! -L $undo && ! -e $probe && ! -L $probe ]]
available=$(df -P -k /root/recovery-private | tail -1)
read -ra fields <<< "$available"
[[ ${fields[3]} =~ ^[0-9]+$ ]]
(( fields[3] > 1572864 )) # One GiB undo plus at least512MiB margin.
ulimit -f 1048576
limit=$(grep '^Max file size' /proc/$$/limits)
read -ra fields <<< "$limit"
[[ ${fields[3]} == 1073741824 && ${fields[4]} == 1073741824 ]]
printf 'UNDO_LIMIT %s\n' "$limit"
owned_probe=no
trap '[[ $owned_probe != yes ]] || rm -- "$probe"' EXIT
set -C
printf '%1048576s' '' > "$probe"
owned_probe=yes
set +C
sync "$probe"
[[ $(stat -Lc %s "$probe") == 1048576 && $(stat -Lc %a "$probe") == 600 ]]
printf 'UNDO_WRITE_FSYNC_TEST bytes=1048576 mode=600\n'
rm -- "$probe"
owned_probe=no
read -r guardsha rest < <(sha256sum /usr/bin/block-check)
[[ $guardsha == 02a2270ad8e0f480efc7464b26d2216fec8a915aa12755da6339015abed0f41c ]]
/usr/bin/block-check /dev/sda2 8 2
[[ ! -e $undo && ! -L $undo ]]
if [[ $1 == --preflight ]]; then
    printf 'PRECHECK_READY undo_absent=yes RAM_parent=46 mode=700 cap_bytes=1073741824\n'
    exit 0
fi
# No automatic prompt answers, optimization, discard, -y or directory reindexing.
exec e2fsck -f -E fixes_only,nodiscard -z "$undo" /dev/sda2
