#!/usr/bin/python3
"""Provision fixed public identity links for the strictly sandboxed agent.

Only package configuration calls this helper; it accepts no runtime requests.
All parent traversal is descriptor-relative and refuses writable/unowned parents.
"""
import contextlib
import os
import pathlib
import secrets
import stat
import subprocess

FLAGS = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC
STATE = '/var/lib/ngfw-system-identity'
FILES = ('hostname', 'issue', 'issue.net', 'motd', 'localtime')
LIMIT = 1024 * 1024


def trusted(fd):
    metadata = os.fstat(fd)
    if metadata.st_uid != 0 or metadata.st_mode & 0o022:
        raise PermissionError('identity parent must be root-owned and not group/world writable')


@contextlib.contextmanager
def directory(root, path, create=False):
    """Root is fixed '/' in production; injected only by isolated fixtures."""
    fd = os.open(root, FLAGS)
    try:
        trusted(fd)
        for name in pathlib.PurePosixPath(path).parts[1:]:
            if name in ('.', '..'):
                raise ValueError('invalid fixed path')
            created = False
            if create:
                try:
                    os.mkdir(name, 0o755, dir_fd=fd)
                    created = True
                    # Persist each ancestor before any public /etc link can
                    # refer to it; syncing only the leaf is insufficient.
                    os.fsync(fd)
                except FileExistsError:
                    pass
            child = os.open(name, FLAGS, dir_fd=fd)
            os.close(fd)
            fd = child
            trusted(fd)
            if created:
                os.fchmod(fd, 0o755)
                os.fsync(fd)
        yield fd
    finally:
        os.close(fd)


def inspect(parent, name):
    try:
        metadata = os.stat(name, dir_fd=parent, follow_symlinks=False)
    except FileNotFoundError:
        return None
    if stat.S_ISLNK(metadata.st_mode):
        return ('link', os.readlink(name, dir_fd=parent))
    if not stat.S_ISREG(metadata.st_mode) or metadata.st_nlink != 1:
        raise ValueError('identity target must be a regular single-link file')
    fd = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=parent)
    try:
        actual = os.fstat(fd)
        if (actual.st_dev, actual.st_ino) != (metadata.st_dev, metadata.st_ino):
            raise ValueError('identity file changed during migration')
        trusted(fd)
        with os.fdopen(fd, 'rb', closefd=False) as stream:
            content = stream.read(LIMIT + 1)
        if len(content) > LIMIT:
            raise ValueError('identity file exceeds migration limit')
        return ('file', content)
    finally:
        os.close(fd)


def put(parent, name, value):
    temporary = '.ngfw-identity-' + secrets.token_hex(16)
    try:
        if value[0] == 'link':
            os.symlink(value[1], temporary, dir_fd=parent)
        else:
            fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600, dir_fd=parent)
            with os.fdopen(fd, 'wb') as stream:
                stream.write(value[1])
                stream.flush()
                os.fchown(stream.fileno(), 0, 0)
                os.fchmod(stream.fileno(), 0o644)
                os.fsync(stream.fileno())
        os.replace(temporary, name, src_dir_fd=parent, dst_dir_fd=parent)
        os.fsync(parent)
    finally:
        try:
            os.unlink(temporary, dir_fd=parent)
        except FileNotFoundError:
            pass


def valid_zone(target):
    # The renderer later changes only the managed link, never the /etc link.
    return (target.startswith('/usr/share/zoneinfo/') and
            str(pathlib.PurePosixPath(target)) == target and
            '..' not in pathlib.PurePosixPath(target).parts)


def provision(root='/'):
    with directory(root, '/etc') as etc, directory(root, STATE, create=True) as state:
        os.fchown(state, 0, 0)
        os.fchmod(state, 0o755)
        # Preflight the entire migration before replacing any /etc object.
        pending = []
        for name in FILES:
            link = STATE + '/' + name
            source = inspect(etc, name)
            existing = inspect(state, name)
            if source == ('link', link):
                if existing is None:
                    raise ValueError('managed identity link has no target')
                if existing[0] == 'link' and (name != 'localtime' or not valid_zone(existing[1])):
                    raise ValueError('unexpected managed identity link')
                continue
            if source is None:
                source = ('link', '/usr/share/zoneinfo/UTC') if name == 'localtime' else ('file', b'')
            if source[0] == 'link' and (name != 'localtime' or not valid_zone(source[1])):
                raise ValueError('refusing to replace an unmanaged identity symlink')
            if existing is not None and existing != source:
                raise ValueError('managed target conflicts with existing host identity')
            pending.append((name, source, link))
        for name, source, link in pending:
            # Commit content first. A crash between these operations is safe and
            # replayable because preflight requires equal source/target data.
            put(state, name, source)
            put(etc, name, ('link', link))
    # Dedicated resolver directory: atomic writes do not need /etc writable.
    with directory(root, '/etc/systemd/resolved.conf.d', create=True):
        pass


def main():
    # Package configuration must not race an old agent that still renders /etc.
    # Existing managed installations are idempotent and need no migration stop.
    with directory('/', '/etc') as etc:
        migration = any(inspect(etc, name) != ('link', STATE + '/' + name) for name in FILES)
    if migration and os.path.exists('/run/systemd/system'):
        result = subprocess.run(['/usr/bin/systemctl', 'is-active', '--quiet', 'ngfw-agent.service'],
                                check=False, timeout=10)
        if result.returncode not in (3, 4):
            raise RuntimeError('stop ngfw-agent before initial identity migration; service state must be inactive')
    provision()


if __name__ == '__main__':
    main()
