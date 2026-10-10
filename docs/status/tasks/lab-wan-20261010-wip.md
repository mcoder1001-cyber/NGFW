# WAN laboratory acceptance WIP

Branch codex/lab-wan-20261010; base 4908716b4501312102382e6979b8fc1ded6f9311.
Remote/local checkpoint da257065c published successfully to origin/codex/lab-wan-20261010. Owned files: envelope, WIP, PPP peer feasibility fixture, evidence and test/topology/multiwan-host-acceptance/run.py.

Completed: parameterize MultiWAN fixture staging to assigned slot20 rather than hardcoded slot7; preserve exact existing assertions and default slot7.
Actual audit .250: ngfw-agent/ngfw-api/vpp active; installed agent/API package source 4c8d1b247c6b, VPP26.06-release+ngfw3; HTTPS health status ok. No pppd/pppoe-server/accel-pppd installed.
Local .195 has real pppd and rp-pppoe server. Legacy PPP live driver predates kernel carrier path and is not acceptance for current implementation.
Initial build failed /tmp inode exhaustion; manager increased inode cap. Three fresh binaries successfully built from4908716b to /tmp/ngfw-lab-wan-20261010/bin. Source guard origin/main PASS23s; carrier contract28tests PASS. Actual genuine Linux peer attempt3 whole PASS: dial/PAP, 3/3 ICMP, peer loss, wrong-password rejection and reconnect. Initial ping readiness race corrected with wait; pool2 changes assigned address so fixture uses one fixed pool. No NGFW carrier/VPP/API/PD acceptance implied. Fixture source saved in lab-wan-20261010-ppp-peer.py. Raw attempts retained. Current blocker: waiting for private VPP budget and API artifact provenance. /dev/shm1.9GiB free. Routing+NAT46 own initial private-VPP budget.
Remaining: fresh current binaries/API artifact and run current-source MultiWAN; assess current carrier PPP real daemon prerequisites.
Next command: NGFW_MULTIWAN_SLOT=20 NGFW_MULTIWAN_EXTENDED=1 NGFW_MULTIWAN_PRODUCT_ROOT=/root/NGFW NGFW_MULTIWAN_BIN_DIR=<fresh-source-bin> python3 test/topology/multiwan-host-acceptance/run.py
