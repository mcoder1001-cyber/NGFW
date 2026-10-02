"""Byte/time bounded argv subprocesses; stop only the group we created."""
import os
from pathlib import Path
import selectors
import signal
import subprocess
import time


class CommandFailed(RuntimeError):
    pass


def run_command(argv, cwd, environment, output, timeout, max_output=4 * 1024 * 1024):
    if (not isinstance(argv, (list, tuple)) or not argv
            or any(not isinstance(part, str) or '\x00' in part for part in argv)
            or not 0 < timeout <= 1200 or type(max_output) is not int or not 1 <= max_output <= 16 * 1024 * 1024):
        raise CommandFailed('invalid argv, timeout or output byte limit')
    fd = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'wb') as log:
        process = subprocess.Popen(argv, cwd=Path(cwd), env=environment, stdout=subprocess.PIPE,
                                   stderr=subprocess.STDOUT, start_new_session=True)
        selector = selectors.DefaultSelector()
        deadline = time.monotonic() + timeout
        written = 0
        try:
            selector.register(process.stdout, selectors.EVENT_READ)
            while selector.get_map():
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise subprocess.TimeoutExpired(argv, timeout)
                for key, _ in selector.select(min(remaining, 0.05)):
                    chunk = os.read(key.fd, 65536)
                    if not chunk:
                        selector.unregister(key.fileobj)
                        continue
                    available = max_output - written
                    log.write(chunk[:available]); written += min(len(chunk), available)
                    if len(chunk) > available:
                        raise CommandFailed('command output exceeded the bounded byte limit')
            status = process.wait(timeout=max(0, deadline - time.monotonic()))
            if status:
                raise CommandFailed(f'command exited {status}; evidence retained in private log')
        finally:
            selector.close()
            try:
                os.killpg(process.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                process.wait(timeout=2)
            except subprocess.TimeoutExpired:
                pass
            # The leader can exit while its child ignores TERM; kill our group.
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.wait(timeout=2)
            process.stdout.close()
    return {'exit_code': status, 'log': str(output), 'output_bytes': written}
