#!/usr/bin/env python3
"""Guest-only supplemental Stop fault wrapper; no production entry point."""
import ctypes
import hashlib
import json
import os
from pathlib import Path
import resource
import signal
import stat
import subprocess
import sys
import time

MARKER = "/run/ngfw-ra-guest-fixture"
MANIFEST = "/run/ngfw-ra-stop-fixture.json"
ELF = "/dev/shm/ngfw-ra-stop-fixture/ra-actual-stop-vm15.test"
ELF_SHA = "9b18c66e92af38a5376d5be5f09e9f02a8fc405e7c7f0797cf2eb140c235fe83"
CGROUP = "/system.slice/ngfw-agent.service"
OUTPUT = "/run/ngfw-ra-stop-coordinator"


class Refused(Exception):
    """Fixed public failure; raw metadata or subprocess output is never printed."""


def require(condition):
    if not condition:
        raise Refused("protected supplemental Stop boundary refused")


def unique(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result)
        result[key] = value
    return result


def protected_fd(path, private=False, executable=False, limit=1048576):
    """Hold a fixed file after no-follow protected parent traversal."""
    parts = Path(path).parts
    require(parts[0] == "/" and all(p not in (".", "..") for p in parts))
    parent = os.open("/", os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for index, name in enumerate(parts[1:-1], 1):
            child = os.open(name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=parent)
            os.close(parent)
            parent = child
            info = os.fstat(parent)
            sticky_shm = parts[:index + 1] == ("/", "dev", "shm") and stat.S_IMODE(info.st_mode) == 0o1777
            require(info.st_uid == 0 and (not info.st_mode & 0o022 or sticky_shm))
        if private:
            info = os.fstat(parent)
            require(stat.S_IMODE(info.st_mode) == 0o700)
        fd = os.open(parts[-1], os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC, dir_fd=parent)
        try:
            info = os.fstat(fd)
            require(stat.S_ISREG(info.st_mode) and info.st_uid == 0 and info.st_nlink == 1 and 0 < info.st_size <= limit)
            require(stat.S_IMODE(info.st_mode) == 0o600 if not executable else info.st_mode & 0o022 == 0 and info.st_mode & 0o111 != 0)
            return fd
        except BaseException:
            os.close(fd)
            raise
    finally:
        os.close(parent)


def private_json(path, limit=1048576):
    fd = protected_fd(path, limit=limit)
    try:
        data = os.read(fd, limit + 1)
        require(len(data) <= limit)
        return json.loads(data, object_pairs_hook=unique)
    finally:
        os.close(fd)


def boot_id():
    return Path("/proc/sys/kernel/random/boot_id").read_text().strip()


def process_birth(pid):
    require(type(pid) is int and pid > 1)
    raw = Path("/proc/" + str(pid) + "/stat").read_text()
    fields = raw[raw.rfind(")") + 2:].split()
    require(len(fields) >= 20)
    return {"BootID": boot_id(), "PID": pid, "StartTime": int(fields[19]),
            "parent": int(fields[1]), "group": int(fields[2]), "session": int(fields[3]), "state": fields[0]}


def same_birth(expected):
    actual = process_birth(expected["PID"])
    require(all(actual[key] == expected[key] for key in ("BootID", "PID", "StartTime")))
    return actual


def signal_verified(pidfd, expected, sig):
    """pidfd prevents PID reuse; fresh complete birth is required before signal."""
    same_birth(expected)
    signal.pidfd_send_signal(pidfd, sig)


def guest_boundary():
    require(os.geteuid() == 0)
    marker = private_json(MARKER, 1024)
    require(set(marker) == {"owner", "bootId"} and marker["owner"] == "ngfw-ra-independent-guest" and marker["bootId"] == boot_id())
    require(os.readlink("/proc/1/exe") in ("/usr/lib/systemd/systemd", "/lib/systemd/systemd"))
    require(os.stat("/proc/1").st_uid == 0)


class Source:
    """Held canonical Source; no scalar PID authorizes resume."""
    def __init__(self, manifest):
        self.identity = manifest["agent"]
        require(set(self.identity) == {"BootID", "PID", "StartTime"})
        require(self.identity["BootID"] == boot_id() and type(self.identity["StartTime"]) is int and self.identity["StartTime"] > 0)
        require(manifest["agentCgroup"] == CGROUP)
        self.device, self.inode = manifest["agentExeDev"], manifest["agentExeIno"]
        self.pidfd = self.image = None
        try:
            require(same_birth(self.identity)["state"] not in ("T", "t"))
            self.verify()
            self.pidfd = os.pidfd_open(self.identity["PID"], 0)
            self.image = os.open("/proc/" + str(self.identity["PID"]) + "/exe", os.O_RDONLY | os.O_CLOEXEC)
            self.verify()
        except BaseException:
            self.close()
            raise

    def verify(self):
        same_birth(self.identity)
        pid = self.identity["PID"]
        require(Path("/proc/" + str(pid) + "/cgroup").read_text() == "0::" + CGROUP + "\n")
        installed = protected_fd("/usr/sbin/ngfw-agent", executable=True, limit=128 * 1024 * 1024)
        actual = os.open("/proc/" + str(pid) + "/exe", os.O_RDONLY | os.O_CLOEXEC)
        try:
            info, running = os.fstat(installed), os.fstat(actual)
            require((info.st_dev, info.st_ino) == (running.st_dev, running.st_ino) == (self.device, self.inode))
            if self.image is not None:
                held = os.fstat(self.image)
                require((held.st_dev, held.st_ino) == (self.device, self.inode))
        finally:
            os.close(actual)
            os.close(installed)
        encoding = self.identity["BootID"] + "/" + str(pid) + "/" + str(self.identity["StartTime"])
        generation = "source-agent-" + hashlib.sha256(encoding.encode()).hexdigest()
        require(os.readlink("/run/ngfw/ra/source-agent-current") == generation)
        record = private_json("/run/ngfw/ra/" + generation + "/identity.json", 1024)
        require(record == {"Source": self.identity, "Version": 1, "Owner": "ngfw-ra-source"})
        same_birth(self.identity)

    def resume(self):
        self.verify()
        require(self.pidfd is not None)
        signal_verified(self.pidfd, self.identity, signal.SIGCONT)

    def close(self):
        failed = False
        for name in ("image", "pidfd"):
            fd = getattr(self, name, None)
            if fd is not None:
                setattr(self, name, None)
                try:
                    os.close(fd)
                except OSError:
                    failed = True
        require(not failed)


def child_limits():
    resource.setrlimit(resource.RLIMIT_FSIZE, (4 * 1024 * 1024, 4 * 1024 * 1024))
    resource.setrlimit(resource.RLIMIT_CORE, (0, 0))


def enable_subreaper():
    # Only this wrapper process adopts its own orphan descendants. No namespace,
    # capability or shared cgroup is changed. Guest gate precedes production use.
    require(ctypes.CDLL(None, use_errno=True).prctl(36, 1, 0, 0, 0) == 0)


def capture_group(child, identity):
    """Capture only an alive own child session, bounded proc scan; never adopt."""
    leader = None
    try:
        leader = same_birth(identity)
        require(leader["parent"] == os.getpid() and leader["group"] == child.pid and leader["session"] == child.pid)
    except (FileNotFoundError, ProcessLookupError):
        require(child.poll() is not None)
    held = []
    try:
        entries = list(Path("/proc").iterdir())
        require(len(entries) <= 4096)
        for entry in entries:
            if not entry.name.isdecimal() or int(entry.name) <= 1:
                continue
            try:
                birth = process_birth(int(entry.name))
                if birth["group"] != child.pid or birth["session"] != child.pid:
                    continue
                # After the leader exits, only a positively adopted own orphan
                # can authorize capture. Group/SID numbers alone are insufficient.
                if leader is None:
                    require(birth["parent"] == os.getpid())
                fd = os.pidfd_open(birth["PID"], 0)
                try:
                    actual = same_birth(birth)
                    require(actual["group"] == child.pid and actual["session"] == child.pid)
                    held.append((fd, birth))
                except BaseException:
                    os.close(fd)
                    raise
            except (FileNotFoundError, ProcessLookupError):
                continue
        if leader is not None:
            try:
                same_birth(identity)
            except (FileNotFoundError, ProcessLookupError):
                require(child.poll() is not None)
                for _, birth in held:
                    try:
                        require(same_birth(birth)["parent"] == os.getpid())
                    except (FileNotFoundError, ProcessLookupError):
                        pass
        return held
    except BaseException:
        for fd, _ in held:
            os.close(fd)
        raise


def stop_child(child, identity):
    child.poll()
    held = capture_group(child, identity)
    try:
        # Only positively captured owned members; no group-ID guessing after exit.
        for fd, birth in held:
            try:
                signal_verified(fd, birth, signal.SIGKILL)
            except (FileNotFoundError, ProcessLookupError):
                pass
        child.wait(timeout=2)
        # Only adopted children with the captured exact PID can be reaped here.
        deadline = time.monotonic() + 2
        for _, birth in held:
            if birth["PID"] != child.pid:
                while True:
                    try:
                        reaped, _ = os.waitpid(birth["PID"], os.WNOHANG)
                        if reaped:
                            break
                        require(time.monotonic() < deadline)
                        time.sleep(0.01)
                    except ChildProcessError:
                        break
    finally:
        for fd, _ in held:
            os.close(fd)


def fixed_elf():
    fd = protected_fd(ELF, private=True, executable=True, limit=128 * 1024 * 1024)
    try:
        digest = hashlib.sha256()
        while data := os.read(fd, 1024 * 1024):
            digest.update(data)
        require(digest.hexdigest() == ELF_SHA)
        return fd
    except BaseException:
        os.close(fd)
        raise


def output_directory():
    try:
        os.mkdir(OUTPUT, 0o700)
    except FileExistsError:
        pass
    info = os.lstat(OUTPUT)
    require(stat.S_ISDIR(info.st_mode) and info.st_uid == 0 and stat.S_IMODE(info.st_mode) == 0o700)
    target = OUTPUT + "/run-" + os.urandom(16).hex()
    os.mkdir(target, 0o700)
    return target


def alarm(*_):
    raise Refused("finite supplemental Stop deadline")


def main():
    require(sys.argv == [sys.argv[0], "--run-guest"])
    guest_boundary()  # No process signal, child or output before this boundary.
    signal.signal(signal.SIGALRM, alarm)
    started = time.monotonic()
    signal.setitimer(signal.ITIMER_REAL, 235)
    source = child = identity = image = log = None
    directory = None
    passed, resumed = False, False
    try:
        manifest = private_json(MANIFEST, 1048576 + 16384)
        source = Source(manifest)
        image = fixed_elf()
        enable_subreaper()
        directory = output_directory()
        os.mkdir(directory + "/tmp", 0o700)
        log = os.open(directory + "/test.log", os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600)
        # Execute the held immutable image, eliminating path replacement races.
        child = subprocess.Popen(["/proc/self/fd/" + str(image), "-test.run=^TestIntegrationActualRAStopFaultBlocksConnectedBeforeVPP$", "-test.count=1", "-test.timeout=220s"],
            cwd="/dev/shm/ngfw-ra-stop-fixture", stdin=subprocess.DEVNULL, stdout=log, stderr=log,
            env={"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LANG": "C", "LC_ALL": "C", "TMPDIR": directory + "/tmp", "NGFW_INTEGRATION": "1", "NGFW_RA_ACTUAL_STOP_GUEST": "1"},
            start_new_session=True, pass_fds=(image,), preexec_fn=child_limits)
        identity = process_birth(child.pid)
        require(identity["parent"] == os.getpid() and identity["group"] == child.pid and identity["session"] == child.pid)
        remaining = 230 - (time.monotonic() - started)
        require(remaining > 0)
        passed = child.wait(timeout=remaining) == 0
    finally:
        # Reserve independent recovery time even when the test is killed/crashes.
        signal.setitimer(signal.ITIMER_REAL, max(0.001, 240 - (time.monotonic() - started)))
        try:
            if child is not None and identity is not None:
                try:
                    stop_child(child, identity)
                except Exception:
                    passed = False
        finally:
            try:
                if source is not None:
                    source.resume()
                    resumed = True
            except Exception:
                passed = False
            finally:
                if source is not None:
                    try:
                        source.close()
                    except Exception:
                        passed = False
                for fd in (image, log):
                    if fd is not None:
                        try:
                            os.close(fd)
                        except OSError:
                            passed = False
        if directory is not None:
            fd = os.open(directory + "/safe-result.json", os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600)
            try:
                os.write(fd, json.dumps({"passed": passed and resumed, "sourceResumed": resumed, "scope": "supplemental-injected-stop", "requiresPublicBaselineRollback": True}).encode())
                os.fsync(fd)
            finally:
                os.close(fd)
        signal.setitimer(signal.ITIMER_REAL, 0)
    require(passed and resumed)
    print("Supplemental guest Stop boundary completed; canonical baseline rollback still required.")


if __name__ == "__main__":
    try:
        main()
    except Exception:
        print("Supplemental guest Stop coordinator refused; private output withheld.", file=sys.stderr)
        raise SystemExit(1)
