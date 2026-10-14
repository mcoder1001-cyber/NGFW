# Remote deployment completion checkpoint

2026-10-14: inspected original script. It only upgrades already-installed NGFW and has no fresh/runtime/bootstrap path. Current main integrated into task branch; original reviewed changes retained.

Implementation in progress: a single compatible entrypoint, isolated package build, verified SSH transfer, explicit fresh/upgrade modes, firstboot and normal API setup. Dedicated build and setup workers are implementing separate modules; parent owns controller and target lifecycle. No tests or live deployment claimed yet.

Next: complete controller/target/fixtures, publish checkpoint, independent review, full unchanged hosted quick and scoped deployment/packaging checks, expected-head merge, main CI. Git CLI push lacks credentials; use authenticated GitHub connector and read-only CLI fetch.
