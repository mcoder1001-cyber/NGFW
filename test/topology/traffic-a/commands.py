"""Bounded fixed-argv subprocesses; stop only the process group we created."""
import os
from pathlib import Path
import signal
import subprocess


class CommandFailed(RuntimeError):
    pass


def run_command(argv, cwd, environment, output, timeout):
    if (not isinstance(argv, (list, tuple)) or not argv
            or any(not isinstance(part, str) or '\x00' in part for part in argv)
            or not 0 < timeout <= 1200):
        raise CommandFailed('invalid argv or timeout')
    # Create a new private log, never overwrite or follow an existing link.
    fd = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'wb') as log:
        process = subprocess.Popen(argv, cwd=Path(cwd), env=environment, stdout=log,
                                   stderr=subprocess.STDOUT, start_new_session=True)
        try:
            status = process.wait(timeout=timeout)
            if status:
                raise CommandFailed(f'command exited {status}; evidence retained in private log')
        except BaseException:
            # Includes timeout/cancellation; descendants are in our new group.
            try:
                os.killpg(process.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                process.wait(timeout=2)
            except subprocess.TimeoutExpired:
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                process.wait(timeout=2)
            raise
    return {'exit_code': status, 'log': str(output)}
