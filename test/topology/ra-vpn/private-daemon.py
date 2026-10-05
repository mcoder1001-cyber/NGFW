#!/usr/bin/env python3
"""Disposable fixture launcher; never a production privileged entry point.

Caller supplies a held host mount namespace as fd3. The actual production Go
helper checks the network/PID/capability/filesystem boundary again before exec.
"""
import fcntl
import json
import os
from pathlib import Path
import subprocess
import sys

PHASE = 0


def refuse():
    raise SystemExit("private RA fixture sandbox refused phase=" + str(PHASE))


def mount(*args):
    subprocess.run(["/usr/bin/mount", *args], check=True,
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def bind(source, target, readonly=True):
    mount("--rbind" if Path(source).is_dir() else "--bind", str(source), str(target))
    if readonly:
        mount("-o", "remount,bind,ro", str(target))


def main():
    global PHASE
    PHASE = 1
    if len(sys.argv) != 5 or os.geteuid() != 0 or os.getpid() != 1:
        refuse()
    instance, artifact, helper, workspace = sys.argv[1:]
    if len(instance) != 64 or any(char not in "0123456789abcdef" for char in instance):
        refuse()
    # NS_GET_NSTYPE: ensure fd3 really pins a mount namespace, then prove this
    # child cannot mount into the caller's shared filesystem view.
    if fcntl.ioctl(3, 0xB703) != 0x20000:
        refuse()
    current = os.stat("/proc/self/ns/mnt")
    original = os.fstat(3)
    if (current.st_dev, current.st_ino) == (original.st_dev, original.st_ino):
        refuse()
    PHASE = 2
    root = Path("/run/ngfw/ra") / instance
    plan = json.loads((root / "network.json").read_text())
    net = os.stat("/proc/self/ns/net")
    if net.st_ino != plan["namespaceInode"] or net.st_ino == plan["hostNamespaceInode"]:
        refuse()
    artifact, helper, workspace = map(lambda value: Path(value).resolve(strict=True),
                                     (artifact, helper, workspace))
    if not all(path.is_relative_to(Path("/dev/shm")) for path in (artifact, helper, workspace)):
        refuse()
    if any(path.stat().st_uid != 0 or path.stat().st_mode & 0o022 for path in (artifact, helper, workspace)):
        refuse()
    PHASE = 3
    for path in artifact.rglob("*"):
        if not path.resolve(strict=True).is_relative_to(artifact):
            refuse()
    mount("--make-rprivate", "/")
    held_root = workspace / "runtime-bind"
    held_root.mkdir(mode=0o700)
    bind(root, held_root, readonly=False)
    public = workspace / "public"
    public.mkdir(mode=0o700)
    for name in ("passwd", "group", "nsswitch.conf", "ld.so.cache", "protocols", "services"):
        (public / name).write_bytes((Path("/etc") / name).read_bytes())
    PHASE = 4
    mount("-t", "tmpfs", "-o", "mode=0755,size=1m", "tmpfs", "/opt")
    Path("/opt/ngfw-ra").mkdir()
    bind(artifact, "/opt/ngfw-ra")
    Path("/opt/ngfw-ra-helper").touch(mode=0o700)
    bind(helper, "/opt/ngfw-ra-helper")
    PHASE = 5
    mount("-t", "tmpfs", "-o", "mode=0755,size=1m", "tmpfs", "/run")
    root.mkdir(mode=0o700, parents=True)
    bind(held_root, root)
    bind(held_root / "daemon", root / "daemon", readonly=False)
    PHASE = 6
    mount("-t", "tmpfs", "-o", "mode=0755,size=1m", "tmpfs", "/etc")
    for name in ("passwd", "group", "nsswitch.conf", "ld.so.cache", "protocols", "services"):
        target = Path("/etc") / name
        target.touch(mode=0o644)
        bind(public / name, target)
    for path in ("/var/lib", "/var/log", "/root", "/home", "/boot", "/mnt", "/media", "/data"):
        if Path(path).is_dir():
            mount("-t", "tmpfs", "-o", "mode=0700,size=1m", "tmpfs", path)
    PHASE = 7
    # Bind sources above before hiding the host's /dev/shm and block devices.
    mount("-t", "tmpfs", "-o", "mode=0755,size=1m", "tmpfs", "/dev")
    for name, minor in (("null", 3), ("zero", 5), ("random", 8), ("urandom", 9)):
        os.mknod("/dev/" + name, 0o20666, os.makedev(1, minor))
    # Match systemd PrivateDevices standard fd aliases; nft reads --file -
    # through /dev/stdin inside this private device filesystem.
    os.symlink("/proc/self/fd", "/dev/fd")
    for name, fd in (("stdin", 0), ("stdout", 1), ("stderr", 2)):
        os.symlink("/proc/self/fd/" + str(fd), "/dev/" + name)
    # The optional authenticated CLIENT fixture resolve plugin must write only
    # its private per-instance file. Its upstream fallback invokes resolvconf
    # when that executable exists; refuse that path in the actual masked view.
    if Path("/opt/ngfw-ra/lib/ipsec/plugins/libstrongswan-resolve.so").exists() and os.path.lexists("/sbin/resolvconf"):
        refuse()
    os.close(3)
    print("RA_FIXTURE_PHASE=8", file=sys.stderr, flush=True)
    os.execve("/usr/bin/setpriv", ["setpriv", "--no-new-privs",
              "--bounding-set=-all,+net_admin,+net_bind_service,+ipc_lock", "--",
              "/opt/ngfw-ra-helper", instance],
              {"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LANG": "C", "LC_ALL": "C"})


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError):
        refuse()
