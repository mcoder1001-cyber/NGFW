import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('acceptance', Path(__file__).with_name('host-acceptance.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class GuardTests(unittest.TestCase):
    def test_refuses_owned_resource_collisions(self):
        for namespaces, links in [('ns-w8-native-peer', ''), ('', '2: w8nwan: UP'), ('', '2: w8nlan: UP')]:
            with self.assertRaises(ValueError):
                module.fixture_available(namespaces, links)
        module.fixture_available('ns-w9-test', '2: w9nwan: UP')

    def test_slot_exports_parsed_as_data(self):
        self.assertEqual(module.lab_environment('NGFW_TEST_PREFIX=w8\nNGFW_SLOT=8')['NGFW_SLOT'], '8')
        self.assertEqual(module.lab_environment('export NGFW_TEST_PREFIX=w8')['NGFW_TEST_PREFIX'], 'w8')
        for text in ['export HOME=/tmp\nNGFW_TEST_PREFIX=w8', 'HOME=/tmp\nNGFW_TEST_PREFIX=w8', 'NGFW_TEST_PREFIX=w20']:
            with self.assertRaises(ValueError):
                module.lab_environment(text)


if __name__ == '__main__':
    unittest.main()
