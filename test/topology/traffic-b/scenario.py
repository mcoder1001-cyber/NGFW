"""Wave-B scenario inventory and strict acceptance contract.

A driver exit code alone is never packet, API or rollback acceptance.
"""
from dataclasses import dataclass


class Refused(ValueError):
    pass


def slot_values(slot):
    if type(slot) is not int or slot not in (*range(1, 12), *range(14, 33)):
        raise Refused("allocated developer slot required (12 is CI, 13 reserved)")
    return {"NGFW_SLOT": str(slot), "NGFW_TEST_PREFIX": f"w{slot}",
            "NGFW_HTTP_PORT": str(3000 + slot * 100 if slot < 12 else 10000 + slot * 100),
            "NGFW_WEB_PORT": str(5000 + slot * 100 if slot < 12 else 14000 + slot * 100),
            "NGFW_VPP_TABLE_BASE": str(slot * 1000)}


@dataclass(frozen=True)
class Phase:
    name: str
    driver: str
    packets: tuple
    limitation: str


PHASES = (
    Phase("ipsec", "test/topology/ipsec/run.sh", ("bidirectional-icmp", "bidirectional-tcp", "esp-no-plaintext"),
          "built-in private REST driver; execute current-source acceptance"),
    Phase("wireguard", "test/topology/wireguard/stack.sh", ("handshake", "inner-icmp", "udp-wireguard"),
          "built-in private REST driver; execute current-source acceptance"),
    Phase("gre", "test/topology/tunnels/run.sh", ("inner-icmp", "gre-encapsulation"),
          "built-in private REST driver; execute current-source acceptance"),
    Phase("vxlan", "test/topology/tunnels/run.sh", ("inner-icmp", "vxlan-encapsulation"),
          "built-in private REST driver; execute current-source acceptance"),
    Phase("bgp", "test/topology/bgp/run.sh", ("learned-prefix-icmp", "learned-prefix-tcp"),
          "built-in private REST driver; execute current-source acceptance"),
    Phase("ospf", "test/topology/ospf/run.sh", ("learned-prefix-icmp", "learned-prefix-tcp"),
          "built-in private REST driver; execute current-source acceptance"),
    Phase("dhcp-relay", "test/topology/kea-dhcp-relay/run.sh", ("lease", "discover-giaddr"),
          "built-in private REST driver; execute current-source acceptance"),
    Phase("det44", "test/topology/det44/run.sh", ("smoke",), "requires private globals acceptance window"),
    Phase("dslite", "test/topology/det44/run.sh", ("smoke",), "DS-Lite pool deletion banned D-211; preserve pool"),
    Phase("cnat", "test/topology/det44/run.sh", ("smoke",), "requires private globals acceptance window"),
    Phase("isis", "test/topology/isis-rip/run.sh", ("adjacency-route",), "requires private OSI punt window"),
    Phase("bfd", "test/topology/bgp/run.sh", ("session-up",), "dedicated BFD evidence export required"),
    Phase("unbound", "test/topology/unbound-chrony-syslog/run.sh", ("dns-response",), "namespace DNS evidence export required"),
    Phase("chrony", "test/topology/unbound-chrony-syslog/run.sh", ("sources",), "slot source evidence export required"),
    Phase("syslog", "test/topology/unbound-chrony-syslog/run.sh", ("received-line",), "slot receiver evidence export required"),
)


def accept(phase, result, *, slot, run_id, source_sha):
    """Validate current-run results from an independently reviewed phase adapter.

    This validates the evidence envelope, not the packet bytes themselves. Adapters
    must verify packets and counters locally and independently retain redacted text.
    """
    slot_values(slot)
    if not isinstance(result, dict):
        raise Refused("phase result object required")
    expected = {"phase": phase.name, "slot": slot, "prefix": f"w{slot}",
                "run_id": run_id, "source_sha": source_sha}
    for key, value in expected.items():
        if result.get(key) != value or type(result.get(key)) is not type(value):
            raise Refused("foreign or stale phase identity: " + key)
    if result.get("status") != "passed":
        raise Refused("phase did not pass")
    commit = result.get("commit", {})
    if (commit.get("status") != "applied" or commit.get("notApplied") != []
            or commit.get("unsupported") != [] or type(commit.get("revision")) is not int
            or commit["revision"] <= 0):
        raise Refused("commit did not fully apply")
    packets = result.get("packets")
    if not isinstance(packets, dict) or set(packets) != set(phase.packets) or any(value is not True for value in packets.values()):
        raise Refused("required packet observations missing")
    if result.get("rollback") != {"status": "applied", "owned_residue": []}:
        raise Refused("rollback or owned residue check failed")
    before, after = result.get("shared_vpp_before"), result.get("shared_vpp_after")
    if (not isinstance(before, dict) or set(before) != {"MainPID", "NRestarts"}
            or type(before["MainPID"]) is not int or before["MainPID"] <= 0
            or type(before["NRestarts"]) is not int or before["NRestarts"] < 0 or after != before):
        raise Refused("shared VPP identity changed or unverified")
    if result.get("cleanup") is not True:
        raise Refused("owned daemon and namespace cleanup unverified")
    return result


def private_identity():
    """Observe the socket and VPP process in this new mount namespace."""
    import os
    from pathlib import Path
    raw=os.environ.get('NGFW_TRAFFIC_PRIVATE_VPP_PID','')
    if not raw.isdecimal() or int(raw)<=0:
        raise Refused('observed private VPP process required')
    pid=int(raw)
    mount=os.readlink('/proc/self/ns/mnt')
    if mount==os.readlink('/proc/1/ns/mnt') or mount!=os.readlink(f'/proc/{pid}/ns/mnt'):
        raise Refused('VPP and probe are not in the same private mount namespace')
    if Path(os.readlink(f'/proc/{pid}/exe')).name!='vpp':
        raise Refused('private process is not VPP')
    api=Path(os.environ.get('NGFW_VPP_API_SOCKET','/run/vpp/api.sock'))
    if str(api)=='/run/vpp/api.sock' or not os.path.samefile(api,'/run/vpp/api.sock'):
        raise Refused('mounted socket is not the observed private VPP socket')
    return {'pid':pid,'mount_namespace':mount,'api_socket':str(api)}
