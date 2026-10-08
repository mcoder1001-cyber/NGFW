"""Run carrier helper controls in the unchanged strict packaging fixture gate."""
import importlib.util
from pathlib import Path


def load_tests(loader, tests, pattern):
    root = Path(__file__).resolve().parents[4]
    source = root / 'scripts/tests/pppoe-kernel-carrier.py'
    spec = importlib.util.spec_from_file_location('ngfw_pppoe_carrier_controls', source)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return loader.loadTestsFromModule(module)
