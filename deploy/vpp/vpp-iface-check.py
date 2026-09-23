#!/usr/bin/env python3
"""vpp-iface-check.py NAME... — exit 0 iff every NAME is a VPP interface (F-startup-gen).

Asks VPP over its binary API (sw_interface_dump with a name filter, exact match on
interface_name), never trusts `vppctl` exit codes. Prints the missing names and exits 1 when
any is absent, 2 when VPP cannot be reached. Used by deploy/vpp/apply-startup.sh; read only.
"""
import os
import sys


def main(names):
    try:
        from vpp_papi import VPPApiClient  # python3-vpp-api, installed with VPP
    except ImportError as e:  # pragma: no cover - host without the package
        print(f"vpp-iface-check: python3-vpp-api missing: {e}", file=sys.stderr)
        return 2
    client = VPPApiClient(server_address=os.environ.get("VRX_VPP_API_SOCKET", "/run/vpp/api.sock"))
    try:
        client.connect("vrx-startup-check")
    except Exception as e:  # noqa: BLE001 - any connect failure means "VPP not healthy"
        print(f"vpp-iface-check: cannot connect to VPP: {e}", file=sys.stderr)
        return 2
    missing = []
    try:
        for name in names:
            found = client.api.sw_interface_dump(name_filter_valid=True, name_filter=name)
            if not any(d.interface_name == name for d in found):  # name_filter is a substring match
                missing.append(name)
    finally:
        client.disconnect()
    if missing:
        print("missing: " + " ".join(missing))
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
