# Independent management acceptance review

Reviewed commit `8f48382973bba707dadcbdd4e3af31a8b7e22cba` read-only, including driver, wrapper and recorded output. Reviewer owns neither management product implementation nor acceptance driver.

Verdict: **APPROVE for the documented live TLS and dataplane API acceptance scope**. No blocking findings.

The driver runs real production API/agent processes in a private VPP namespace on slot 9. It verifies actual certificate peers across a hot certificate change, TLS 1.2 refusal with minimum TLS 1.3, unchanged API PID, invalid/mismatched/expired certificate rejection, semantic dataplane validation pointers, preview SHA and unchanged installed startup file. Its leak check covers response bodies, audit rows and API logs. Cleanup targets only spawned processes and the slot database/runtime. Recorded evidence distinguishes these cases from browser coverage, which remains unavailable; this approval does not assert browser acceptance or complete feature definition of done.
