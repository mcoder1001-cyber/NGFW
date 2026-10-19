#!/usr/bin/env python3
"""Generate the packages' maintainer scripts with the real debhelper; never install a package.

A private source tree gets debian/control, debian/changelog and the hand-written
*.postinst scripts, plus every unit file each *.install line places under
usr/lib/systemd/system/. dh_installsystemd runs with the exact arguments of the
debian/rules override, dh_installdeb merges the snippets, and the DEBIAN/* scripts
come back as text. Scripts can then be executed against recording fakes of
deb-systemd-invoke, deb-systemd-helper and systemctl (D-253).
"""
import os
import pathlib
import re
import shlex
import shutil
import subprocess
import tempfile

SOURCE = pathlib.Path(__file__).resolve().parents[1]
ROOT = SOURCE.parents[2]
# Where the repository keeps the unit files prepare.sh stages (checked by basename).
UNIT_DIRECTORIES = ('deploy/systemd', 'deploy/vpp', 'deploy/upgrade', 'deploy/support-bundle',
                    'scripts/pppoe-carrier-assets')
UNIT_DESTINATION = 'usr/lib/systemd/system/'
SCRIPTS = ('preinst', 'prerm', 'postinst', 'postrm')
TOOLS = ('dh_installsystemd', 'dh_installdeb')
# [Unit] settings whose explicit stop or restart propagates from the named unit to the
# configuring unit (systemd.unit(5): Requires=, Requisite=, BindsTo= "this unit will be
# stopped (or restarted) if one of the other units is explicitly stopped (or restarted)";
# PartOf= "limited to stopping and restarting"). Wants=, After= and Before= never do.
DEPENDENCY_KEYS = ('Requires', 'Requisite', 'BindsTo', 'PartOf')
# [Install] setting that adds the same edge, from each named unit to the shipped unit,
# once the shipped unit is enabled (a <name>.requires/ symlink, as deploy/upgrade/provision
# and ngfw-upgrade create for ngfw-upgrade-prepare.service).
INSTALL_REQUIRED_BY = 'RequiredBy'


def missing_tools():
    return [tool for tool in TOOLS if shutil.which(tool) is None]


def packages():
    control = (SOURCE / 'debian/control').read_text()
    return re.findall(r'^Package: (\S+)$', control, re.M)


def installsystemd_arguments():
    """The arguments debian/rules passes to dh_installsystemd (one command line)."""
    rules = (SOURCE / 'debian/rules').read_text()
    lines = re.findall(r'^override_dh_installsystemd:\n((?:\t.*\n)+)', rules, re.M)
    if len(lines) != 1:
        raise AssertionError('debian/rules must carry exactly one dh_installsystemd override')
    commands = [line[1:] for line in lines[0].splitlines()]
    if len(commands) != 1 or not commands[0].startswith('dh_installsystemd '):
        raise AssertionError('the dh_installsystemd override must be one dh_installsystemd command: ' + repr(commands))
    return shlex.split(commands[0])[1:]


def install_lines(package):
    for line in (SOURCE / f'debian/{package}.install').read_text().splitlines():
        line = line.split('#', 1)[0].strip()
        if line:
            source, destination = line.split()
            yield source, destination


def locate_unit(source, name):
    """A staged unit when the prepared tree exists, else the repository file of that name."""
    staged = SOURCE / source
    if staged.is_file():
        return staged
    found = [ROOT / directory / name for directory in UNIT_DIRECTORIES if (ROOT / directory / name).is_file()]
    if len(found) != 1:
        raise AssertionError(f'unit {name} must exist exactly once under {UNIT_DIRECTORIES}, found {found}')
    return found[0]


def shipped_units(package):
    """(unit name, source file) for every unit a package installs into usr/lib/systemd/system/."""
    units = []
    for source, destination in install_lines(package):
        if destination == UNIT_DESTINATION:
            name = pathlib.PurePosixPath(source).name
            units.append((name, locate_unit(source, name)))
    return units


def shipped_dropins(package):
    """(target unit, drop-in file) for every drop-in a package installs into <unit>.d/."""
    dropins = []
    for source, destination in install_lines(package):
        match = re.fullmatch(re.escape(UNIT_DESTINATION) + r'([^/]+)\.d/', destination)
        if match:
            dropins.append((match.group(1), SOURCE / source))
    return dropins


def maintscript_units(package):
    """The units dh_installsystemd generates start/stop snippets for: no templates."""
    return sorted(name for name, _ in shipped_units(package) if '@' not in name)


def unit_settings(text):
    """[(section, key, value)] for every assignment of a unit file or drop-in, in file order;
    comment and blank lines are skipped and the section is None before the first header."""
    settings, section = [], None
    for raw in text.splitlines():
        line = raw.strip()
        if not line or line[0] in '#;':
            continue
        if line.startswith('[') and line.endswith(']'):
            section = line[1:-1]
            continue
        key, separator, value = line.partition('=')
        if separator:
            settings.append((section, key.strip(), value.strip()))
    return settings


def unit_setting(text, key, section='Unit'):
    """The unit names a space-separated list setting accumulates, in order; an empty
    assignment resets the list as systemd does."""
    names = []
    for found_section, found_key, value in unit_settings(text):
        if found_section == section and found_key == key:
            names = names + value.split() if value else []
    return names


def unit_dependencies(text):
    """Units named by the [Unit] dependency settings whose explicit stop or restart propagates
    to the configuring unit (DEPENDENCY_KEYS)."""
    names = set()
    for key in DEPENDENCY_KEYS:
        names.update(unit_setting(text, key))
    return names


def stop_propagation_closure(start, enabled=False, overrides=None):
    """Every unit whose explicit stop (or restart) propagates to `start` through the shipped
    unit files and drop-ins (systemd.unit(5) Requires=), excluding `start` itself. With
    `enabled`, the [Install] RequiredBy= edges of the shipped units count as well, as they do
    once those units are enabled. `overrides` ({unit name: text}) replaces shipped unit texts,
    for negative controls."""
    edges = {}
    for package in packages():
        for name, path in shipped_units(package):
            text = (overrides or {}).get(name) or path.read_text()
            edges.setdefault(name, set()).update(unit_dependencies(text))
            if enabled:
                for dependent in unit_setting(text, INSTALL_REQUIRED_BY, 'Install'):
                    edges.setdefault(dependent, set()).add(name)
        for target, path in shipped_dropins(package):
            edges.setdefault(target, set()).update(unit_dependencies(path.read_text()))
    closure, frontier = set(), [start]
    while frontier:
        for dependency in edges.get(frontier.pop(), ()):
            if dependency not in closure:
                closure.add(dependency)
                frontier.append(dependency)
    closure.discard(start)
    return closure


def generate(arguments=None):
    """{package: {script: text}} as dh_installsystemd + dh_installdeb generate them."""
    missing = missing_tools()
    if missing:
        raise RuntimeError('debhelper tools missing: ' + ' '.join(missing))
    arguments = installsystemd_arguments() if arguments is None else list(arguments)
    with tempfile.TemporaryDirectory() as temporary:
        tree = pathlib.Path(temporary) / 'ngfw'
        debian = tree / 'debian'
        debian.mkdir(parents=True)
        for name in ('control', 'changelog'):
            shutil.copyfile(SOURCE / 'debian' / name, debian / name)
        for script in (SOURCE / 'debian').glob('*.postinst'):
            shutil.copyfile(script, debian / script.name)
        for package in packages():
            unit_directory = debian / package / UNIT_DESTINATION
            unit_directory.mkdir(parents=True)
            for name, path in shipped_units(package):
                shutil.copyfile(path, unit_directory / name)
        # Inside dpkg-buildpackage dh exports DH_INTERNAL_OPTIONS/DH_INTERNAL_OVERRIDE to its
        # override targets; a nested debhelper call would inherit them (-a/-i/-p restrict the
        # packages it processes), so the fixture runs with debhelper's defaults only.
        env = {key: value for key, value in os.environ.items() if not key.startswith('DH_')}
        env.update(LC_ALL='C.UTF-8', DH_QUIET='1')
        for command in (['dh_installsystemd', *arguments], ['dh_installdeb']):
            result = subprocess.run(command, cwd=tree, env=env, capture_output=True, text=True)
            if result.returncode != 0:
                raise AssertionError(f'{command[0]} failed ({result.returncode}): {result.stderr}')
        generated = {}
        for package in packages():
            control = debian / package / 'DEBIAN'
            if shipped_units(package) and not control.is_dir():
                raise AssertionError(f'{package} got no maintainer scripts (build architecture mismatch?)')
            generated[package] = {name: (control / name).read_text() for name in SCRIPTS
                                  if (control / name).is_file()}
        return generated


def debhelper_sections(text):
    """The bodies of the '# Automatically added by dh_installsystemd' blocks of a script."""
    return re.findall(r'# Automatically added by dh_installsystemd/[^\n]*\n(.*?)# End automatically added section',
                      text, re.S)


class Recorder:
    """Runs a generated script with recording fakes on PATH; /run/systemd/system becomes a
    fixture directory so the systemd-only branches execute on any host."""

    FAKE = '#!/bin/sh\nprintf "%s\\n" "$*" >> "$FIXTURE_LOG_DIR/$(basename "$0")"\n'

    def __init__(self, directory):
        self.root = pathlib.Path(directory)
        self.bin = self.root / 'bin'
        self.logs = self.root / 'logs'
        self.systemd = self.root / 'run/systemd/system'
        for path in (self.bin, self.logs, self.systemd):
            path.mkdir(parents=True, exist_ok=True)
        for tool in ('deb-systemd-invoke', 'deb-systemd-helper', 'systemctl'):
            fake = self.bin / tool
            fake.write_text(self.FAKE)
            fake.chmod(0o755)

    def run(self, text, *arguments):
        """Execute a script; return {tool: [recorded argument lines]} for the fakes it called."""
        for log in self.logs.iterdir():
            log.unlink()
        script = self.root / 'script'
        script.write_text(text.replace('/run/systemd/system', str(self.systemd)))
        env = {key: value for key, value in os.environ.items() if key != 'DPKG_ROOT'}
        env['PATH'] = str(self.bin) + os.pathsep + env.get('PATH', '/usr/bin:/bin')
        env['FIXTURE_LOG_DIR'] = str(self.logs)
        result = subprocess.run(['sh', str(script), *arguments], env=env, capture_output=True, text=True)
        if result.returncode != 0:
            raise AssertionError(f'script {" ".join(arguments)} failed ({result.returncode}): {result.stderr}')
        return {log.name: log.read_text().splitlines() for log in sorted(self.logs.iterdir())}


def stopped_units(calls):
    """Unit names passed to `deb-systemd-invoke stop …` in recorded calls."""
    units = set()
    for line in calls.get('deb-systemd-invoke', []):
        words = shlex.split(line)
        if words and words[0] == 'stop':
            units.update(words[1:])
    return units


# dpkg maintainer-script invocations of an upgrade (dpkg(1), "Unpack" and "Configure"):
# old-prerm upgrade, new-preinst upgrade, old-postrm upgrade, new-postinst configure,
# and the error paths new-prerm failed-upgrade / old-preinst abort-upgrade / new-postrm abort-upgrade.
UPGRADE_PATHS = {
    'preinst': (('upgrade', '0.1.0~dev+old'), ('abort-upgrade', '0.1.0~dev+new')),
    'prerm': (('upgrade', '0.1.0~dev+new'), ('failed-upgrade', '0.1.0~dev+old'),
              ('deconfigure', 'in-favour', 'ngfw-meta', '0.1.0~dev+new')),
    'postrm': (('upgrade', '0.1.0~dev+new'), ('abort-upgrade', '0.1.0~dev+old'), ('failed-upgrade', '0.1.0~dev+old')),
}
