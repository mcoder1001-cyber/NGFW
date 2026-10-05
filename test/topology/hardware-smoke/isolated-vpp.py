#!/usr/bin/env python3
"""Run a test command with a disposable VPP mapped onto /run/vpp in a private mount namespace.

Only the child namespace sees the socket mapping. The system VPP is never stopped
or reconfigured. The command's exit code and the disposable VPP's survival count.
"""
import os
from pathlib import Path
import subprocess
import signal
import sys
import time

ROOT = Path(__file__).resolve().parents[3]


def main():
    if len(sys.argv) < 2:
        raise SystemExit('usage: isolated-vpp.py <command> [args...]')
    if sys.argv[1] != '--child':
        raise SystemExit(subprocess.call(['unshare', '--mount', '--propagation', 'private',
                                         sys.executable, str(Path(__file__).resolve()), '--child', *sys.argv[1:]]))
    command = sys.argv[2:]
    os.environ['NGFW_DISPOSABLE_VPP'] = '1'
    def terminate(signum, frame):
        raise SystemExit(128 + signum)
    signal.signal(signal.SIGTERM, terminate)
    runtime = ROOT / '.scratch' / ('isolated-vpp-' + str(os.getpid()))
    runtime.mkdir(parents=True)
    subprocess.run(['mount', '--bind', str(runtime), '/run/vpp'], check=True)
    workers = ' workers 5-6' if os.environ.get('NGFW_ISOLATED_WORKERS') == '2' else ''
    if os.environ.get('NGFW_ISOLATED_TEST_RUN') == '1':
        test_run = runtime / 'test-run'
        test_run.mkdir(mode=0o711)
        subprocess.run(['mount', '--bind', str(test_run), '/run/ngfw-test'], check=True)
        prefix = test_run / os.environ['NGFW_TEST_PREFIX']
        prefix.mkdir(mode=0o711)
    conf = runtime / 'startup.conf'
    plugin_path = os.environ.get('NGFW_ISOLATED_PLUGIN_PATH', '')
    plugin_directive = f'path {plugin_path}' if plugin_path else ''
    conf.write_text(f'''unix {{ nodaemon cli-listen /run/vpp/cli.sock log /run/vpp/vpp.log }}
api-segment {{ prefix fulltest{os.getpid()} }}
socksvr {{ default }}
statseg {{ socket-name /run/vpp/stats.sock }}
cpu {{ main-core 4{workers} }}
dpdk {{ no-pci }}
plugins {{ {plugin_directive} plugin linux_cp_plugin.so {{ enable }} plugin linux_nl_plugin.so {{ enable }} plugin npt66_plugin.so {{ enable }} }}
''')
    with (runtime / 'process.log').open('w') as output:
        vpp = subprocess.Popen(['vpp', '-c', str(conf)], stdout=output, stderr=subprocess.STDOUT)
        try:
            for _ in range(200):
                if vpp.poll() is not None:
                    print((runtime / 'process.log').read_text(), file=sys.stderr)
                    raise SystemExit(2)
                if (runtime / 'api.sock').exists() and (runtime / 'cli.sock').exists():
                    break
                time.sleep(.1)
            else:
                raise SystemExit('disposable VPP startup timed out')
            print(f'DISPOSABLE_VPP pid={vpp.pid} runtime={runtime}', flush=True)
            child_env=os.environ.copy()
            if child_env.get('NGFW_TRAFFIC_B_REST')=='1':
                child_env['NGFW_TRAFFIC_PRIVATE_VPP_PID']=str(vpp.pid)
                child_env['NGFW_VPP_API_SOCKET']=str(runtime/'api.sock')
            result = subprocess.call(command,env=child_env)
            if vpp.poll() is not None:
                print(f'DISPOSABLE_VPP_CRASH exit={vpp.returncode}', file=sys.stderr)
                result = result or 1
            raise SystemExit(result)
        finally:
            if vpp.poll() is None:
                vpp.terminate()
                try: vpp.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    vpp.kill(); vpp.wait()
            print(f'DISPOSABLE_VPP_STOPPED pid={vpp.pid}', flush=True)


if __name__ == '__main__':
    main()
