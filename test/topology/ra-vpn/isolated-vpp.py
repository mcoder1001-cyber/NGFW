#!/usr/bin/env python3
"""Run only RA packet acceptance under independently verified private VPP/net/mount namespaces."""
import fcntl
import os
from pathlib import Path
import signal
import stat
import subprocess
import sys
import tempfile
import time


def owned(path):
    path = Path(path).resolve(strict=True)
    info = path.stat()
    if not path.is_relative_to(Path('/dev/shm')) or info.st_uid != 0 or info.st_mode & 0o022:
        raise SystemExit('private VPP runtime parent refused')
    return path


def main():
    mode = os.environ.get('NGFW_RA_TEST_MODE', 'packet')
    if mode not in ('packet','production'):
        raise SystemExit('fixed private test mode required')
    if os.geteuid() != 0:
        raise SystemExit('root required for disposable namespaces')
    if len(sys.argv) == 2 and sys.argv[1] == '--child':
        # The outer launcher supplies held original namespace descriptors. No
        # network/daemon command can run if unshare failed or did not isolate.
        net_fd = int(os.environ['NGFW_RA_VPP_NET_FD'])
        mount_fd = int(os.environ['NGFW_RA_VPP_MOUNT_FD'])
        if net_fd == mount_fd or not all(3 <= fd <= 1024 for fd in (net_fd, mount_fd)):
            raise SystemExit('original namespace handles refused')
        for fd, kind, name in ((net_fd, 0x40000000, 'net'), (mount_fd, 0x20000, 'mnt')):
            if fcntl.ioctl(fd, 0xb703) != kind:
                raise SystemExit('wrong original namespace handle')
            original = os.fstat(fd)
            current = os.stat('/proc/self/ns/' + name)
            if (original.st_dev, original.st_ino) == (current.st_dev, current.st_ino):
                raise SystemExit('private VPP namespace isolation refused')
            os.close(fd)
        runtime = owned(os.environ['NGFW_RA_VPP_RUNTIME'])
        subprocess.run(['/usr/bin/mount', '--make-rprivate', '/'], check=True)
        subprocess.run(['/usr/bin/mount', '--bind', str(runtime), '/run/vpp'], check=True)
        prefix = 'ra19_' + str(os.getpid())
        conf = runtime / 'startup.conf'
        conf.write_text(f'''unix {{ nodaemon cli-listen /run/vpp/cli.sock log /run/vpp/vpp.log }}
api-segment {{ prefix {prefix} }}
socksvr {{ socket-name /run/vpp/api.sock }}
statseg {{ socket-name /run/vpp/stats.sock }}
cpu {{ main-core 4 }}
memory {{ main-heap-size 256M main-heap-page-size default }}
buffers {{ buffers-per-numa 16384 page-size default }}
plugins {{ plugin default {{ disable }} plugin tap_plugin.so {{ enable }} plugin acl_plugin.so {{ enable }} plugin ping_plugin.so {{ enable }} plugin af_packet_plugin.so {{ enable }} plugin dhcp_plugin.so {{ enable }} plugin l3xc_plugin.so {{ enable }} plugin adl_plugin.so {{ enable }} plugin urpf_plugin.so {{ enable }} plugin svs_plugin.so {{ enable }} plugin linux_cp_plugin.so {{ enable }} plugin abf_plugin.so {{ enable }} plugin ikev2_plugin.so {{ enable }} plugin wireguard_plugin.so {{ enable }} }}
''')
        os.chmod(conf, 0o600)
        with (runtime / 'vpp-process.log').open('xb') as log:
            process = subprocess.Popen(['/usr/bin/vpp', '-c', str(conf)], stdout=log, stderr=subprocess.STDOUT)
            try:
                for _ in range(150):
                    if process.poll() is not None:
                        raise SystemExit('own disposable VPP startup failed; bounded diagnostics private')
                    if all((runtime / name).is_socket() for name in ('api.sock', 'cli.sock')):
                        break
                    time.sleep(0.1)
                else:
                    raise SystemExit('own disposable VPP startup timed out')
                env = dict(os.environ, NGFW_RA_PRIVATE_VPP='1', NGFW_AGENT_VPP_SOCKET='/run/vpp/api.sock')
                test_log = runtime / 'go-test.log'
                with test_log.open('xb') as test_output:
                    os.chmod(test_log, 0o600)
                    package, name = ('./internal/agent','^TestIntegrationPrivateProductionRAControllerLifecycle$') if mode=='production' else ('./internal/ra_vpn','^TestIntegrationPrivateVPPPolicyEAP$')
                    command = ['go', 'test', package, '-run', name, '-count=1', '-v']
                    test_binary = os.environ.get('NGFW_RA_TEST_BIN')
                    test_cwd = None
                    if test_binary:
                        binary = owned(test_binary)
                        if not binary.is_file() or binary.stat().st_nlink != 1 or not os.access(binary, os.X_OK):
                            raise SystemExit('private precompiled test binary refused')
                        command = [str(binary), '-test.run=' + name, '-test.count=1', '-test.v', '-test.timeout=3m']
                        test_cwd = Path.cwd() / package
                    if mode == 'production':
                        # Match the protected agent unit: its NSFS bind mounts must
                        # be visible to the separately mounted VPP/manager parent.
                        command = ['/usr/bin/unshare', '--mount', '--propagation', 'private', '--'] + command
                    result = subprocess.call(command, cwd=test_cwd, env=env, stdout=test_output, stderr=subprocess.STDOUT)
                for line in test_log.read_text(errors='replace').splitlines():
                    if line.startswith(('=== RUN', '--- PASS', '--- FAIL', 'PASS', 'FAIL', 'ok ')) or '_test.go:' in line:
                        print(line[:400], flush=True)
                if process.poll() is not None:
                    result = result or 1
                raise SystemExit(result)
            finally:
                if process.poll() is None:
                    process.terminate()
                    try:
                        process.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait()
        return
    parent = owned(os.environ['NGFW_RA_EVIDENCE_ROOT'])
    runtime = Path(tempfile.mkdtemp(prefix='private-vpp-', dir=parent))
    (runtime / '.owner').write_text('F-ra-vpn /root/upgrade slot19 private VPP\n')
    os.environ['NGFW_RA_VPP_RUNTIME'] = str(runtime)
    net = os.open('/proc/self/ns/net', os.O_RDONLY)
    mount = os.open('/proc/self/ns/mnt', os.O_RDONLY)
    try:
        # Preserve the exact opened NSFS handles; inherited limiter descriptors
        # may occupy lower numbers and must never be overwritten or closed.
        os.environ['NGFW_RA_VPP_NET_FD'] = str(net)
        os.environ['NGFW_RA_VPP_MOUNT_FD'] = str(mount)
        command = ['/usr/bin/unshare', '--net', '--mount', '--fork', '--', sys.executable, str(Path(__file__).resolve()), '--child']
        raise SystemExit(subprocess.call(command, pass_fds=(net, mount)))
    finally:
        os.close(net)
        os.close(mount)


if __name__ == '__main__':
    main()
