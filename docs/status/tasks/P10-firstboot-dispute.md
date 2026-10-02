# P10 firstboot third verification routing

D-156 process: a third review round goes to an independent arbiter. Developer and reviewer agree on the remaining failure; this is process escalation, not a factual dispute.

Initial firstboot review found unreachable post-marker credential cleanup and unvalidated/random JWT persistence. Cleanup is resolved. Round two reproduced a valid 64-hex JWT followed by an empty duplicate assignment being accepted because shell command substitution strips trailing newlines. systemd environment last assignment can then be empty, causing runtime signing keys to change after restart.

Required invariant: exactly one assignment with exactly 64 lowercase hexadecimal characters; retain bootstrap credentials and do not publish completion on every malformed/random-generation failure. Verify against the actual package unit and API environment consumer. Next code fix must have independent A1/security/process arbitration and unchanged full hosted quick before merge. Full P10 remains partial; no appliance/lab PASS is claimed.
