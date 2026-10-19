# D-239: fixed RA IPC runtime boundary

The independent strongSwan engine approved by the owner requires manager-created root-owned IPC sockets. Actual isolated Boot17 observed the same publisher socket inode as root:root mode 0600 before the original agent started and root:ngfw mode 0600 afterwards. The strict receiver correctly refused the changed ownership (stage 12, no deadline expiration).

Place the fixed publisher, namespace broker, numeric target suppliers and numeric observers under `/run/ngfw-ra-ipc`, outside the agent unit RuntimeDirectory=ngfw. The contract derives numeric socket names only from validated kernel PIDs. Retain UID 0, GID 0, mode 0600, protected parents, PID1 peer authentication, fresh boot/image/capability validation and bounded operations. No old-path fallback is accepted. Instance, source-reference and namespace state paths remain unchanged.

The original agent service and hardening stay byte-identical. Fixed publisher socket startup also requires the fixed namespace broker listener; neither socket activation starts an engine tunnel. This is a scoped correction within the approved independent-engine design, with no added agent privilege or relaxed trust boundary.

Acceptance requires independent source/security review and a fresh immutable guest bundle proving first activation and restart, followed by actual production API and packet lifecycle tests. Historical failures remain evidence; this decision does not claim runtime acceptance.
