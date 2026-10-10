#!/bin/bash
# Read-only proof collector; never repairs, changes namespaces of PID1, or mounts.
set -u
export LC_ALL=C PATH=/usr/sbin:/usr/bin:/sbin:/bin
olddev=2050 # dev_t for verified ext4 root major:minor8:2
failures=0
races=0
processes=0
fds=0
nsfs=0
declare -A checked_namespaces
fail() { printf 'REFUSE %s\n' "$*"; failures=$((failures+1)); }
if ! rootdev=$(stat -Lc %d /); then fail 'current root device unavailable'; exit 2; fi
printf 'AUDIT rootdevice=%s target=8:2 parentdisk=8:0\n' "$rootdev"
[[ $rootdev != "$olddev" ]] || fail 'current root is original root'
for p in /proc/[0-9]*; do
    [[ -d $p ]] || continue
    pid=${p##*/}
    if ! raw=$(cat "$p/stat" 2>/dev/null); then
        [[ -d $p ]] && fail "unreadable process stat pid=$pid" || races=$((races+1))
        continue
    fi
    rest=${raw##*) }
    read -ra fields <<< "$rest"
    flags=${fields[6]:-0}
    kernel=$(( (flags & 2097152) != 0 ))
    processes=$((processes+1))
    if ! namespace=$(readlink "$p/ns/mnt" 2>/dev/null) || [[ $namespace != 'mnt:['*']' ]]; then
        if (( kernel )); then namespace=kernel-none;
        elif [[ -d $p ]]; then fail "unreadable mount namespace pid=$pid"; namespace=unreadable;
        else races=$((races+1)); namespace=vanished; fi
    fi
    printf 'PROCESS pid=%s kernel=%s namespace=%s\n' "$pid" "$kernel" "$namespace"
    if (( !kernel )); then
        for name in root cwd exe; do
            ref=$p/$name
            if meta=$(stat -Lc 'dev=%d inode=%i type=%F' "$ref" 2>/dev/null); then
                printf 'REF pid=%s kind=%s %s path=%s\n' "$pid" "$name" "$meta" "$(readlink "$ref" 2>/dev/null)"
                dev=${meta%% *}; dev=${dev#dev=}
                [[ $dev != "$olddev" ]] || fail "oldroot $name pid=$pid"
            elif [[ -d $p ]]; then fail "unreadable $name pid=$pid"; else races=$((races+1)); fi
        done
        if [[ -r $p/maps ]]; then
            while IFS= read -r line; do
                read -ra fields <<< "$line"
                [[ ${fields[3]:-} != 08:02 ]] || fail "oldroot mapping pid=$pid"
            done < "$p/maps"
        elif [[ -d $p ]]; then fail "unreadable maps pid=$pid"; fi
    fi
    if [[ -r $p/mountinfo ]]; then
        mount_entries=0
        while IFS= read -r line; do
            mount_entries=$((mount_entries+1))
            read -ra fields <<< "$line"
            [[ ${fields[2]:-} != 8:2 ]] || fail "oldroot mount pid=$pid"
            [[ $line != *' - nsfs '* ]] || fail "bound nsfs requires separate review pid=$pid"
        done < "$p/mountinfo"
        (( kernel || mount_entries > 0 )) || fail "empty user-process mountinfo pid=$pid"
    elif (( !kernel )) && [[ -d $p ]]; then fail "unreadable mountinfo pid=$pid"; fi
    if [[ ! -r $p/fd ]] && (( !kernel )) && [[ -d $p ]]; then fail "unreadable fd directory pid=$pid"; fi
    for fd in "$p"/fd/*; do
        [[ -L $fd ]] || continue
        number=${fd##*/}
        if meta=$(stat -Lc 'dev=%d rdev=%t:%T inode=%i type=%F' "$fd" 2>/dev/null); then
            fds=$((fds+1))
            mountid=''
            if [[ -r $p/fdinfo/$number ]]; then
                while read -r key value extra; do
                    [[ $key != mnt_id: ]] || mountid=$value
                done < "$p/fdinfo/$number"
            elif [[ -L $fd ]]; then fail "unreadable fdinfo pid=$pid fd=$number"; fi
            printf 'FD pid=%s fd=%s %s mnt_id=%s path=%s\n' "$pid" "$number" "$meta" "$mountid" "$(readlink "$fd" 2>/dev/null)"
            dev=${meta%% *}; dev=${dev#dev=}
            [[ $dev != "$olddev" ]] || fail "oldroot fd pid=$pid fd=$number"
            rdev=${meta#* rdev=}; rdev=${rdev%% *}
            [[ $rdev != 8:2 && $rdev != 8:0 ]] || fail "open root/disk device pid=$pid fd=$number"
            if ! fstype=$(stat -fLc %T "$fd" 2>/dev/null); then
                [[ -L $fd ]] && fail "unreadable fd filesystem pid=$pid fd=$number" || races=$((races+1))
                continue
            fi
            if [[ $fstype == nsfs ]]; then
                nsfs=$((nsfs+1))
                inode=${meta#* inode=}; inode=${inode%% *}
                [[ $inode =~ ^[0-9]+$ ]] || { fail "invalid namespace inode pid=$pid fd=$number"; continue; }
                if [[ -z ${checked_namespaces[$inode]+present} ]]; then
                    checked_namespaces[$inode]=1
                    /usr/bin/nsfs-check "$fd" 8 2
                    result=$?
                    (( result == 0 )) || fail "namespace fd inspection pid=$pid fd=$number exit=$result"
                else
                    printf 'NAMESPACE_REFERENCE inode=%s already_inspected=yes\n' "$inode"
                fi
            fi
        elif [[ -L $fd && -d $p ]]; then fail "unreadable fd target pid=$pid fd=$number"; else races=$((races+1)); fi
    done
done
[[ -d /sys/class/block/sda/holders && -d /sys/class/block/sda2/holders ]] || fail 'sysfs block holder directories unavailable'
for path in /sys/class/block/sda/holders/* /sys/class/block/sda2/holders/*; do
    [[ ! -e $path ]] || fail "block holder ${path##*/}"
done
printf 'SUMMARY processes=%s fds=%s nsfs=%s races=%s failures=%s\n' "$processes" "$fds" "$nsfs" "$races" "$failures"
(( races == 0 )) || fail 'process/fd disappearance makes this audit inconclusive'
(( failures == 0 )) || exit 2
# Namespace helper children have all exited before the exclusive final probe.
/usr/bin/block-check /dev/sda2 8 2
result=$?
printf 'FINAL_BLOCK_GUARD exit=%s\n' "$result"
exit "$result"
