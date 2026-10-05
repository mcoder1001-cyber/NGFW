# F-ha-state-sync — independent R2 redirect closure

Reviewed exact local source `48e5af12fd6a42323f3c83d04129eb6563cef650`, published source `4cf795488ae93480330f174281f29bd675d589f6`. Reviewer branch: `codex/review-r2-security-closures-20261005`; own executable RAM worktree `/dev/shm/r2-closures`. Owned files: this report and the separate BFD security closure report; no product edits.

The compatibility-driver Bearer redirect BLOCKER reported on review checkpoint `1c71a8959640d5ace6d42acf5bbca8e6ffe34f6d` is CLOSED. `fetch` constructs the shared `acceptance.Api` before a request: credential-free HTTPS origin validation rejects HTTP, userinfo, paths, queries and fragments; `NoRedirect` refuses redirects before a second request. Initial requests retain the intended Bearer header. Responses are limited to 1 MiB, require HTTP 200, and failed requests/JSON decoding withhold response content. Neither observation nor resync bypasses this fetch path. Standard urllib TLS verification remains enabled.

Actual independent commands in the exact-source worktree:

```
python3 -m unittest discover -s test/topology/ha-state-sync -p 'test_*.py' -v
Ran 16 tests in 3.242s
OK
```

Additionally ran an independent Python probe using the REAL urllib opener/HTTP 302 dispatch, a private in-memory HTTPSHandler response, and NoRedirect (not a mocked direct redirect callback). Both Location values, foreign HTTPS and HTTP downgrade, raised `Refused('API redirect refused')`; the initial request carried its expected Bearer header. Output:

```
actual urllib opener 302 dispatch: foreign HTTPS and HTTP downgrade refused; Bearer preserved only initial request
```

No network request, live HA fault, shared service change, VPP restart or RA-engine test was run. This is targeted verification of the repaired security finding; previous full HA security review provenance remains the cited checkpoint. Complete quick CI and the other applicable panel reviews remain manager prerequisites. No new BLOCKER or MAJOR in the inspected affected path.

Verdict: **APPROVE** (R2 security closure only).
