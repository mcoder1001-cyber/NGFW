#!/usr/bin/python3
"""Provision fixed API storage directories without following untrusted links."""
import contextlib
import grp
import os
import pathlib
import pwd

FLAGS = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC


def open_child(parent, name):
    try:
        os.mkdir(name, mode=0o755, dir_fd=parent)
    except FileExistsError:
        pass
    # An existing link, including a replacement after mkdir, must fail here.
    return os.open(name, FLAGS, dir_fd=parent)


@contextlib.contextmanager
def directory(path):
    parts = pathlib.PurePath(path).parts
    if not parts or parts[0] != '/' or '..' in parts:
        raise ValueError('storage path must be absolute without parent traversal')
    descriptor = os.open('/', FLAGS)
    try:
        for name in parts[1:]:
            child = open_child(descriptor, name)
            os.close(descriptor)
            descriptor = child
        yield descriptor
    finally:
        os.close(descriptor)


def permissions(descriptor, uid, gid):
    # Never resolve a pathname again after opening: a concurrent rename/link
    # swap can only leave this descriptor on the original directory inode.
    os.fchown(descriptor, uid, gid)
    os.fchmod(descriptor, 0o750)


def provision(api, data, uid, gid):
    with directory(api) as descriptor:
        permissions(descriptor, uid, gid)
    # The parent also contains privileged shared upgrade/config state. API owns
    # only the three fixed children, so it cannot rename/replace root state.
    with directory(data) as descriptor:
        permissions(descriptor, 0, gid)
        for name in ('backups', 'updates', 'support'):
            child = open_child(descriptor, name)
            try:
                permissions(child, uid, gid)
            finally:
                os.close(child)


if __name__ == '__main__':
    provision('/var/lib/ngfw/api', '/data',
              pwd.getpwnam('ngfw').pw_uid, grp.getgrnam('ngfw').gr_gid)
