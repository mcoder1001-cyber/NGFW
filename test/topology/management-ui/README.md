# Management host acceptance

Provision an isolated real slot stack/database with baseline API TLS on HTTP port + 1,
a dedicated admin API key, and the PID of that test's API process. The driver checks
rotation by a fresh validated TLS handshake, fingerprint/revision, TLS 1.2 refusal,
a mismatch problem pointer, public response/audit scrubbing and unchanged API process
start time. It restores baseline TLS and deletes the three temporary uploaded secrets.
Generated PEM/key files remain in a private temporary directory and are removed on exit.
Never use a production database. The manager drops the slot database after acceptance.

```sh
NGFW_INTEGRATION=1 NGFW_MANAGEMENT_UI_HOST=1 python3 test/topology/management-ui/run.py --slot 14
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s test/topology/management-ui -v
```

Environment: NGFW_HOST_ACCESS_TOKEN holds the dedicated admin API key;
NGFW_HOST_API_PID identifies the manager-provisioned test API; TMPDIR should point
to the worker's directory on the root filesystem when /tmp inodes are full.
The driver neither provisions host services nor restarts any process. It uses the
shared lab lock and the allocated slot's UI-host lock. Concurrent changes refuse
cleanup rather than discarding another owner's candidate. A dedicated unused API
key is necessary because interactive sessions of one user share a candidate owner.
Browser screenshots/nav/tab checks and API-log scrub are separately owed on the lab.
Helper fixtures/localhost TLS unit tests are never product acceptance proof.
