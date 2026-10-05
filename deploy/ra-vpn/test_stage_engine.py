import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("stage_engine", Path(__file__).with_name("stage-engine.py"))
engine = importlib.util.module_from_spec(spec)
spec.loader.exec_module(engine)


class StageTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.artifact, self.target = self.root / "artifact", self.root / "target"
        self.prefix = self.artifact / "opt/ngfw-ra"
        (self.prefix / "share/ngfw").mkdir(parents=True)
        (self.prefix / "sbin").mkdir()
        for name in ("charon-systemd", "swanctl"):
            (self.prefix / "sbin" / name).write_text("fixture executable\n")
        self.abi = {"format": 1, "architecture": "x86_64", "os": {"ID": "ubuntu", "VERSION_ID": "26.04"}, "runtimePackages": {"libc6": "2.43-x", "libssl3t64": "3.5-x", "libsystemd0": "259-x"}}
        self.receipt = self.prefix / "share/ngfw/engine-abi.json"
        self.receipt.write_text(json.dumps(self.abi))
        (self.target / "etc").mkdir(parents=True)
        (self.target / "etc/os-release").write_text("ID=ubuntu\nVERSION_ID=26.04\n")
        (self.target / "var/lib/dpkg").mkdir(parents=True)
        (self.target / "var/lib/dpkg/status").write_text("\n\n".join(f"Package: {name}\nVersion: {version}\nArchitecture: amd64\nStatus: install ok installed" for name, version in self.abi["runtimePackages"].items()))

    def refuse(self):
        with self.assertRaises(engine.Refused):
            engine.stage(self.artifact, self.target)
        self.assertFalse((self.target / "opt/ngfw-ra").exists())

    def test_matching_offline_abi_stages(self):
        engine.stage(self.artifact, self.target)
        self.assertEqual((self.target / "opt/ngfw-ra/sbin/charon-systemd").read_text(), "fixture executable\n")

    def test_changed_runtime_package_refused_before_creation(self):
        self.abi["runtimePackages"]["libc6"] = "older"
        self.receipt.write_text(json.dumps(self.abi))
        self.refuse()

    def test_wrong_distribution_refused(self):
        (self.target / "etc/os-release").write_text("ID=debian\nVERSION_ID=12\n")
        self.refuse()

    def test_escape_symlink_refused(self):
        (self.prefix / "escape").symlink_to(self.target)
        self.refuse()

    def test_symlinked_target_abi_refused(self):
        status = self.target / "var/lib/dpkg/status"
        status.rename(self.root / "status")
        status.symlink_to(self.root / "status")
        self.refuse()

    def test_host_root_refused(self):
        with self.assertRaises(engine.Refused):
            engine.validate(self.artifact, Path("/"))


if __name__ == "__main__":
    unittest.main()
