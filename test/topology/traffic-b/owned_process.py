"""Bounded cleanup only for sessions created by this fixture's Popen calls."""
import os
import signal
import subprocess
import time


def stop_session(child,grace=15):
    # Caller must have created child with start_new_session=True. A surviving
    # group keeps its leader PID reserved, even after the leader has exited.
    try:os.killpg(child.pid,signal.SIGTERM)
    except ProcessLookupError:return
    deadline=time.monotonic()+grace
    if child.poll() is None:
        try:child.wait(timeout=grace)
        except subprocess.TimeoutExpired:pass
    while time.monotonic()<deadline:
        try:os.killpg(child.pid,0)
        except ProcessLookupError:return
        time.sleep(.05)
    try:os.killpg(child.pid,signal.SIGKILL)
    except ProcessLookupError:pass
    if child.poll() is None:child.wait(timeout=5)
