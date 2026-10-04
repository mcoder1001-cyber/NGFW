"""Driver unit checks only; these do not prove live firewall enforcement."""
import contextlib
import importlib.util
import io
import pathlib
import subprocess
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("autoblock_driver", pathlib.Path(__file__).with_name("run.py"))
driver = importlib.util.module_from_spec(spec)
spec.loader.exec_module(driver)


class DriverTests(unittest.TestCase):
    def scenario(self, detector="webLogin", status=401, ssh_error=b"Permission denied (publickey)."):
        blocked = set()
        hits = {}
        calls = []
        attacker, allowed = "192.0.2.3", "192.0.2.4"

        def fail(address):
            hits[address] = hits.get(address, 0) + 1
            if hits[address] >= 10 and address != allowed:
                blocked.add(address)

        def request(origin, path, address=None, data=None, token=None):
            calls.append(path)
            if path == "/state/auto-block":
                return 200, {"items": [{"source": a + "/32"} for a in blocked]}
            if path == "/actions/auto-block/unblock":
                blocked.remove(data["source"])
                return 200, {"unblocked": True}
            if status == 401:
                fail(address)
            return status, {}

        def ssh(argv, **kwargs):
            if b"Permission denied" in ssh_error:
                fail(argv[argv.index("-b") + 1])
            self.assertIn("StrictHostKeyChecking=yes", argv)
            self.assertTrue(kwargs["timeout"] <= 8)
            return subprocess.CompletedProcess(argv, 255, stderr=ssh_error)

        argv = ["run.py", "--api", "https://ngfw.example/api/v1", "--control-address", "192.0.2.2",
                "--client-address", attacker, "--allow-address", allowed, "--local-host", "192.0.2.1",
                "--through-host", "198.51.100.1", "--through-port", "8080", "--block-sec", "60",
                "--removal", "manual", "--detector", detector]
        if detector == "ssh":
            argv += ["--ssh-key", "/tmp/test-key", "--ssh-known-hosts", "/tmp/test-known-hosts"]
        with patch("sys.argv", argv), patch.dict(driver.os.environ, {"NGFW_TEST_API_TOKEN": "test-only"}), \
                patch.object(driver, "request", request), \
                patch.object(driver, "probe", lambda address, *args: address not in blocked), \
                patch.object(driver.subprocess, "run", ssh), patch.object(driver.time, "sleep"), \
                contextlib.redirect_stdout(io.StringIO()):
            driver.main()
        self.assertIn("/actions/auto-block/unblock", calls)
        self.assertEqual(hits, {attacker: 10, allowed: 10})
        self.assertEqual(blocked, set())

    def test_web_authentication_manual_removal_and_allowlist(self):
        self.scenario()

    def test_ssh_authentication_manual_removal_and_allowlist(self):
        self.scenario(detector="ssh")

    def test_http_rate_limit_is_not_authentication_failure(self):
        with self.assertRaisesRegex(AssertionError, "did not reach authentication: 429"):
            self.scenario(status=429)

    def test_ssh_host_key_failure_is_not_authentication_failure(self):
        with self.assertRaisesRegex(AssertionError, "before authentication"):
            self.scenario(detector="ssh", ssh_error=b"Host key verification failed.")


if __name__ == "__main__":
    unittest.main()
