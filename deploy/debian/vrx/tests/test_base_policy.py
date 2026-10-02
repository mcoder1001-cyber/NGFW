#!/usr/bin/env python3
"""Pure static-policy and shipped dependency checks; no netlink/ruleset load."""
import importlib.util
import pathlib
import unittest

SOURCE = pathlib.Path(__file__).resolve().parents[1]
ROOT = SOURCE.parents[2]
SPEC = importlib.util.spec_from_file_location('base_policy', SOURCE / 'assets/render-base-policy.py')
POLICY = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(POLICY)


class BasePolicy(unittest.TestCase):
    def test_no_dataplane_devices_has_empty_typed_punt_set(self):
        rendered = POLICY.render('ens192', '')
        self.assertIn('set punt_interfaces { type ifname;', rendered)
        self.assertNotIn('elements =', rendered)
        self.assertIn('policy drop', rendered)
        self.assertIn('iifname "ens192" tcp dport { 22, 443 } accept', rendered)
        self.assertNotIn('flush ruleset', rendered)
        self.assertNotIn('table inet vrx {', rendered)
        self.assertNotIn('vpp*', rendered)

    def test_explicit_punts_are_sorted_and_invalid_inputs_refuse(self):
        rendered = POLICY.render('mgmt0', 'lan2,lan1')
        self.assertIn('elements = { "lan1", "lan2" }', rendered)
        for management, punts in [('lo', ''), ('mgmt;accept', ''), ('mgmt0', 'mgmt0'),
                                  ('mgmt0', 'lan1,lan1'), ('mgmt0', 'vpp*'), ('mgmt0', 'lan\n0')]:
            with self.subTest(management=management, punts=punts):
                self.assertRaises(ValueError, POLICY.render, management, punts)

    def test_early_generation_has_no_database_dependency_or_distro_order_override(self):
        units = ROOT / 'deploy/systemd'
        if not units.is_dir(): units = SOURCE / 'stage/usr/lib/systemd/system'
        early = (units / 'vrx-firewall-bootstrap.service').read_text()
        firstboot = (units / 'vrx-firstboot.service').read_text()
        nft = (SOURCE / 'assets/nftables-vrx.conf').read_text()
        self.assertIn('DefaultDependencies=no', early)
        self.assertIn('Before=nftables.service', early)
        self.assertNotIn('postgresql', early)
        self.assertIn('nftables.service', firstboot.split('Before=')[0])
        self.assertNotIn('Before=nftables.service', firstboot)
        self.assertIn('Requires=vrx-firewall-bootstrap.service', nft)
        self.assertNotIn('Before=', nft)
        self.assertNotIn('[Install]', nft)
        self.assertNotIn('disable nftables', (SOURCE / 'debian/vrx-meta.postinst').read_text())


if __name__ == '__main__':
    unittest.main()
