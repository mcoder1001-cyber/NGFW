#!/usr/bin/env python3
"""Execute changed shell fragments with harmless stubs; never provision a host."""
import pathlib
import re
import subprocess
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
SOURCE = (ROOT / 'tools/lab').read_text()


class ShellSource(unittest.TestCase):
    def test_deferred_remote_sources_preserve_literal_variables(self):
        for marker, expected in [
            ('REMOTE_VERIFY', '-f=${Version}'),
            ('REMOTE_CYCLE', 'unit="$1"; shift; systemctl "$@" "$unit"'),
        ]:
            code = re.search(r"<<'" + marker + r"'\n(.*?)\n" + marker, SOURCE, re.S).group(1)
            # Execute the actual quoted heredoc with hostile caller variable values.
            result = subprocess.run(['bash', '-c', "Version=caller; unit=caller; cat <<'" + marker + "'\n" + code + '\n' + marker], text=True, capture_output=True, check=True)
            self.assertEqual(result.stdout, code + '\n')
            self.assertIn(expected, result.stdout)

    def test_profile_defers_home_and_path_to_login_shell(self):
        source = (ROOT / 'scripts/20-install-build.sh').read_text()
        code = re.search(r"<<'GO_PROFILE'\n(.*?)\nGO_PROFILE", source, re.S).group(1)
        result = subprocess.run(['bash', '-c', "cat <<'GO_PROFILE'\n" + code + '\nGO_PROFILE'], text=True, capture_output=True, check=True)
        self.assertEqual(result.stdout, 'export PATH=/usr/local/go/bin:$HOME/go/bin:$PATH\n')

    def test_actual_teardown_preserves_failure_fallback_and_argv(self):
        function = re.search(r'^rig_side_down\(\).*?^}', SOURCE, re.M | re.S).group(0)
        stubs = '''
set -euo pipefail
vpp_if_exists() { return 0; }
rig_reset_classify_soft() { printf 'classify reset:%s\\n' "$*" >&2; }
vpp_cli_ok() { printf 'vpp argv:%s\\n' "$*" >&2; return "$VPP_RC"; }
ip() { [[ "$*" != 'netns list' ]] && return 1; return 0; }
sleep() { :; }
echo() { if [[ "$*" == '  delete vpp host-w1l0' ]]; then printf 'success-log\\n'; return "$ECHO_RC"; fi; builtin echo "$@"; }
warn() { printf 'fallback:%s\\n' "$*"; }
'''
        for vpp_rc, echo_rc, expected in [(0, 0, 'success-log\n'), (1, 0, 'fallback:rig: VPP could not delete host-w1l0\n'), (0, 1, 'success-log\nfallback:rig: VPP could not delete host-w1l0\n')]:
            result = subprocess.run(['bash', '-c', stubs + '\n' + function + f'\nVPP_RC={vpp_rc}; ECHO_RC={echo_rc}; rig_side_down ns-w1-lan w1l0 host-w1l0'], text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout, expected)
            self.assertEqual(result.stderr, 'classify reset:host-w1l0\nvpp argv:delete host-interface name w1l0\n')

    def test_leftover_format_preserves_each_line(self):
        result = subprocess.run(['bash', '-c', "left=$'host-w1l0\\nns-w1-lan'; printf '  LEFTOVER %s\\n' \"${left//$'\\n'/$'\\n  LEFTOVER '}\""], text=True, capture_output=True, check=True)
        self.assertEqual(result.stdout, '  LEFTOVER host-w1l0\n  LEFTOVER ns-w1-lan\n')


if __name__ == '__main__':
    unittest.main()
