# Dataplane host acceptance
Against a manager-provisioned real slot API/agent, with dedicated admin API key in
NGFW_HOST_ACCESS_TOKEN (never argv), run:

```sh
NGFW_INTEGRATION=1 NGFW_DATAPLANE_UI_HOST=1 python3 test/topology/dataplane-ui/run.py --slot 14
python3 -m unittest discover -s test/topology/dataplane-ui -v
```
Only an initially clean slot candidate is edited and discarded on exit. The driver never commits
startup settings or restarts VPP. Real runtime, worker diff/digest, apply availability boolean, problem+json
field pointer and unchanged installed startup/restart counter are required.
Unit fixtures provide no live proof. Browser screenshot/nav/apply checks remain owed on same endpoint.
