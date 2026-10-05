#!/usr/bin/env python3
"""Stage a private build only into a matching Ubuntu appliance offline root."""
import json
from pathlib import Path
import shlex
import shutil
import sys


class Refused(ValueError):
    pass


def release(path):
    values = {}
    for line in path.read_text().splitlines():
        if "=" in line and not line.startswith("#"):
            key, raw = line.split("=", 1)
            words = shlex.split(raw)
            if len(words) == 1:
                values[key] = words[0]
    return {key: values.get(key) for key in ("ID", "VERSION_ID")}


def packages(path):
    values = {}
    for paragraph in path.read_text().split("\n\n"):
        fields = dict(line.split(": ", 1) for line in paragraph.splitlines()
                      if ": " in line and not line.startswith(" "))
        if fields.get("Status") == "install ok installed" and fields.get("Architecture") == "amd64":
            values[fields.get("Package")] = fields.get("Version")
    return values


def validate(artifact, target):
    artifact, target = artifact.resolve(strict=True), target.resolve(strict=True)
    root_stat, target_stat = Path("/").stat(), target.stat()
    if (root_stat.st_dev, root_stat.st_ino) == (target_stat.st_dev, target_stat.st_ino):
        raise Refused("shared host root refused")
    prefix = artifact / "opt/ngfw-ra"
    if prefix.is_symlink() or (artifact / "opt").is_symlink():
        raise Refused("private engine prefix symlink refused")
    destination = target / "opt/ngfw-ra"
    if destination.exists() or destination.is_symlink() or (target / "opt").is_symlink():
        raise Refused("new private engine destination required")
    manifest = prefix / "share/ngfw/engine-abi.json"
    if manifest.is_symlink() or manifest.stat().st_size > 16384:
        raise Refused("bounded ABI receipt required")
    abi = json.loads(manifest.read_text())
    expected = {"ID": "ubuntu", "VERSION_ID": "26.04"}
    for relative in ("etc", "etc/os-release", "var", "var/lib", "var/lib/dpkg", "var/lib/dpkg/status"):
        if (target / relative).is_symlink():
            raise Refused("target ABI path symlink refused")
    if abi.get("format") != 1 or abi.get("architecture") != "x86_64" or abi.get("os") != expected or release(target / "etc/os-release") != expected:
        raise Refused("Ubuntu 26.04 amd64 target ABI required")
    actual = packages(target / "var/lib/dpkg/status")
    required = abi.get("runtimePackages", {})
    if set(required) != {"libc6", "libssl3t64", "libsystemd0"} or any(not value or actual.get(name) != value for name, value in required.items()):
        raise Refused("target runtime library packages differ from builder ABI")
    for path in prefix.rglob("*"):
        if not path.resolve(strict=True).is_relative_to(prefix):
            raise Refused("engine path escapes private prefix")
        if not path.is_file() and not path.is_dir():
            raise Refused("unsupported engine artifact entry")
    if not (prefix / "sbin/charon-systemd").is_file() or not (prefix / "sbin/swanctl").is_file():
        raise Refused("engine executables missing")
    return prefix, destination


def stage(artifact, target):
    prefix, destination = validate(artifact, target)
    destination.parent.mkdir(mode=0o755, exist_ok=True)
    shutil.copytree(prefix, destination, symlinks=True)


if __name__ == "__main__":
    try:
        if len(sys.argv) != 3:
            raise Refused("usage: stage-engine.py ARTIFACT_ROOT OFFLINE_APPLIANCE_ROOT")
        stage(Path(sys.argv[1]), Path(sys.argv[2]))
    except (OSError, ValueError, KeyError) as error:
        print("RA engine staging refused: " + str(error), file=sys.stderr)
        sys.exit(1)
