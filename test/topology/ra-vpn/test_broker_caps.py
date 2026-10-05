#!/usr/bin/env python3
"""Finite private namespace capability and SCM_RIGHTS boundary regression."""
import array
import ctypes
import fcntl
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import unittest


def verify_caps(expected):
    values = dict(line.split(':', 1) for line in Path('/proc/self/status').read_text().splitlines() if ':' in line)
    for field in ('CapEff', 'CapPrm', 'CapBnd'):
        assert int(values[field].strip(), 16) == expected
    for field in ('CapInh', 'CapAmb'):
        assert int(values[field].strip(), 16) == 0
    assert values['NoNewPrivs'].strip() == '1'


def bounded(arguments, caps):
    return ['/usr/bin/setpriv', '--no-new-privs', '--bounding-set=-all,' + caps,
            '--inh-caps=-all', '--ambient-caps=-all',
            '/usr/bin/python3', str(Path(__file__).resolve()), *arguments]


def broker(path):
    verify_caps((1 << 21) | (1 << 18))
    assert os.stat('/proc/self/ns/mnt').st_ino != os.stat('/proc/1/ns/mnt').st_ino
    listener = socket.socket(socket.AF_UNIX, socket.SOCK_SEQPACKET)
    listener.bind(path)
    os.chmod(path, 0o600)
    listener.listen(1)
    listener.settimeout(5)
    peer, _ = listener.accept()
    peer.settimeout(5)
    raw, ancillary, flags, _ = peer.recvmsg(2048, socket.CMSG_SPACE(12), socket.MSG_CMSG_CLOEXEC)
    assert flags & ~socket.MSG_CMSG_CLOEXEC == 0
    request = json.loads(raw)
    credentials = peer.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12)
    import struct
    pid, uid, _ = struct.unpack('3i', credentials)
    assert uid == 0 and pid == request['pid']
    received = array.array('i')
    assert len(ancillary) == 1 and ancillary[0][:2] == (socket.SOL_SOCKET, socket.SCM_RIGHTS)
    received.frombytes(ancillary[0][2])
    assert len(received) == 3
    try:
        for fd, kind, inode in zip(received, (0x20000, 0x40000000, 0x40000000), request['inodes']):
            assert fcntl.ioctl(fd, 0xb703) == kind and os.fstat(fd).st_ino == inode
        try:
            foreign = os.open('/proc/' + str(pid) + '/fd/' + str(request['original']), os.O_RDONLY)
        except PermissionError:
            pass
        else:
            os.close(foreign)
            raise AssertionError('broker unexpectedly bypassed source ptrace boundary')
        # A stat success is not proof of an NSFS inode: under these bounds the
        # kernel exposes the proc symlink inode, not the target namespace.
        observed_namespace = os.stat('/proc/' + str(pid) + '/ns/mnt')
        assert observed_namespace.st_ino != request['source_mount']
        for suffix in ('exe', 'root'):
            try:
                os.stat('/proc/' + str(pid) + '/' + suffix)
            except PermissionError:
                pass
            else:
                raise AssertionError('broker unexpectedly bypassed process filesystem boundary')
        try:
            os.readlink('/proc/' + str(pid) + '/exe')
        except PermissionError:
            pass
        else:
            raise AssertionError('broker unexpectedly bypassed executable ptrace boundary')
        libc = ctypes.CDLL(None, use_errno=True)
        assert libc.unshare(0x200) == 0
        assert libc.setns(received[0], 0x20000) == 0
        assert os.stat('/proc/thread-self/ns/mnt').st_ino == request['inodes'][0]
        peer.send(b'OK')
    finally:
        for fd in received:
            os.close(fd)
        peer.close()
        listener.close()


def agent(path, mount, host):
    verify_caps((1 << 12) | (1 << 21) | (1 << 14))
    # Acquire after dropping to the actual agent bounds, not beforehand.
    try:
        manager = os.open('/proc/1/ns/mnt', os.O_RDONLY)
    except PermissionError:
        manager = None
    else:
        try:
            try:
                fcntl.ioctl(manager, 0xb703)
            except OSError as error:
                import errno
                assert error.errno == errno.ENOTTY
            else:
                raise AssertionError('unexpected independent typed manager namespace acquisition')
        finally:
            os.close(manager)
    private = os.open('/proc/self/ns/net', os.O_RDONLY)
    peer = socket.socket(socket.AF_UNIX, socket.SOCK_SEQPACKET)
    peer.settimeout(5)
    for _ in range(100):
        try:
            peer.connect(path)
            break
        except FileNotFoundError:
            import time
            time.sleep(0.01)
    else:
        raise AssertionError('bounded private broker did not listen')
    fds = array.array('i', (mount, host, private))
    request = json.dumps({'pid': os.getpid(), 'original': private, 'source_mount': os.stat('/proc/self/ns/mnt').st_ino, 'inodes': [os.fstat(fd).st_ino for fd in fds]}).encode()
    peer.sendmsg([request], [(socket.SOL_SOCKET, socket.SCM_RIGHTS, fds)])
    assert peer.recv(16) == b'OK'
    peer.close()
    os.close(private)


def coordinator():
    assert os.geteuid() == 0
    assert os.stat('/proc/self/ns/mnt').st_ino != os.stat('/proc/1/ns/mnt').st_ino
    assert os.stat('/proc/self/ns/net').st_ino != os.stat('/proc/1/ns/net').st_ino
    with tempfile.TemporaryDirectory(prefix='ra-broker-caps-', dir='/dev/shm/r19t') as temporary:
        os.chmod(temporary, 0o700)
        path = str(Path(temporary) / 'broker.sock')
        mount = os.open('/proc/self/ns/mnt', os.O_RDONLY)
        host = os.open('/proc/self/ns/net', os.O_RDONLY)
        receiver = subprocess.Popen(['/usr/bin/unshare', '--mount', '--propagation', 'private', '--',
                                     *bounded(['--broker', path], '+sys_admin,+sys_chroot')])
        try:
            subprocess.run(['/usr/bin/unshare', '--net', '--mount', '--propagation', 'private', '--',
                            *bounded(['--agent', path, str(mount), str(host)], '+net_admin,+sys_admin,+ipc_lock')],
                           pass_fds=(mount, host), check=True, timeout=8)
            assert receiver.wait(timeout=3) == 0
        finally:
            if receiver.poll() is None:
                receiver.kill()
                receiver.wait(timeout=3)
            os.close(mount)
            os.close(host)
    print('exact agent/broker capabilities, denied proc FD access, typed SCM_RIGHTS and private mount setns PASS')


class BrokerCaps(unittest.TestCase):
    def test_exact_bounds_handoff_and_setns(self):
        result = subprocess.run(['/usr/bin/unshare', '--net', '--mount', '--propagation', 'private', '--',
                                 '/usr/bin/python3', str(Path(__file__).resolve()), '--coordinator'],
                                capture_output=True, text=True, timeout=15)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('private mount setns PASS', result.stdout)


if __name__ == '__main__':
    if sys.argv[1:] == ['--coordinator']:
        coordinator()
    elif len(sys.argv) == 3 and sys.argv[1] == '--broker':
        broker(sys.argv[2])
    elif len(sys.argv) == 5 and sys.argv[1] == '--agent':
        agent(sys.argv[2], int(sys.argv[3]), int(sys.argv[4]))
    else:
        unittest.main()
