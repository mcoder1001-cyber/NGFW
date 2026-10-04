# Auto-block live topology acceptance

`run.py` performs real HTTP login failures and TCP packet probes. It is a lab-only acceptance driver, not a unit mock.
Run from a host with three bindable IPv4 sources: allowlisted control, unallowlisted attacker, and another allowlisted
source. The attacker path must traverse VPP towards a listening TCP server, and also reach the NGFW host HTTPS port.
The control API connection must remain outside the attacker block. Configure webLogin threshold10/window60,
blockSec60, maxBlockSec300, and include both control/allow sources in security.autoBlock.allowlist. Start with a clean
first-offence attacker (remove previous rows or choose a new lab source). Keep valid TLS trust for the control origin.

Supply a test administrator access token in `NGFW_TEST_API_TOKEN` without committing or printing it. Then run:

```sh
python3 test/topology/autoblock/run.py --api https://ngfw.example/api/v1 \
  --control-address 10.7.77.2 --client-address 10.7.77.3 --allow-address 10.7.77.4 \
  --local-host 10.7.77.1 --through-host 10.7.78.2 --through-port 8080 --block-sec 60
```

The driver proves both paths work first, performs ten failed logins, waits for the source to enter the API set,
checks local-in and forwarded TCP are cut off, waits for TTL expiry, checks both paths reopen, and repeats failures
from the allowlisted source to prove both paths stay open. It never changes configuration or restarts VPP/daemons.
Run under the lab's shared lock and assigned slot as required by `docs/lab/shared-host-rules.md`. Existing lab rig
provisioning and listening services are prerequisites; this script does not bypass their ownership boundaries.

No live topology execution has been performed in the cloud workspace; acceptance is NOTRUN until output from the
real lab is attached. `python3 -m py_compile` only verifies driver syntax.
