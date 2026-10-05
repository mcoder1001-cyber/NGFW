# Independent private HTTP transport review

Verdict APPROVE source scope. Reviewed remote
74c4ecd59f27796a4d58747a218136f816845d85, exact tree
77b546088be354336be2d707e181e84033372974 (local37741e67dd58539dbd2263fa18b8f3bfa67bbae5).
Read-only review of delta since earlier approved security46470888af.

Shared private_http.py builds an opener with empty proxy configuration and
redirect handler refusing every redirect, preserving default TLS verification.
No requests or credentials are issued at import. Existing consumers retain HTTP
status/HTTPError paths, API role/candidate operations and live opt-in guards;
only their HTTP transport and safe helper import change. Capture HTTP errors
now close response bodies through context management. No credential logging,
new dependency, framework change or product privilege expansion introduced.

Independent actual verification:

- python3 -B -m unittest discover -s test/topology -p test_private_http.py -v:
  4 tests PASS in0.896s. Real loopback origins/sinks cover301/302/303/307/308
  withGET/POST, ambient proxy rejection, unchanged200/409 handling, three
  existing slot clients and private capture download. Foreign sinks/proxy
  receive zero Authorization-bearing requests.
- python3 -B -m unittest discover -s test/topology/capture-trace -v:
  9 tests PASS in0.007s, including private/disposable/slot/BPF guards and
  transport timeout/status preservation.
- Diff whitespace validation PASS.

No host/global mutations or full gate run. Complete unchanged mandatory gates
and current integration tree remain merge requirements. Tests prove redirect/
proxy refusal for the reviewed acceptance clients, not product-wide absence of
unknown HTTP flaws or full deployment security acceptance.
