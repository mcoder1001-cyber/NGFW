# Auto-block detector attribute extension

Existing main contract: EVENT_KIND_AUTOBLOCK_OBSERVED = 25 with Event.attributes map,
source_ip (validated host source) and detector (ssh, vpnAuth, portScan). No enum/message
reshaping or generated-code edit is needed.

Additive map attribute for portScan: destination_port, canonical decimal TCP/UDP
destination port in 1..65535. The API ignores portScan events lacking a valid port,
rather than falsely counting duplicate ports as distinct. SSH/VPN remain unchanged.
The agent forwards each trusted kernel probe after journal-cursor replay rejection;
the API counts and refreshes distinct ports within its configured sliding window.
This closes a preexisting false-negative: repeated probes to a port refreshed the
host activity but not the first-observation timestamp used by the API threshold.
