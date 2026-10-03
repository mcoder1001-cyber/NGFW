# Capture contract and stop/recovery update

- Existing additive protobuf contract: CaptureAction drop field 7 and error_filter field 8;
  CaptureList, CaptureRead, CaptureDelete. No new CaptureStop RPC.
- Snaplen 0 means 9000 (historical 65535 default superseded).
- Action emits a started line and done metadata; pcap_chunk is never sent.
- Files are retained privately; unsafe files produce state error. Start is admin-only.
- POST /api/v1/actions/capture/{id}/stop is admin-only and audited, status 202.
- Stop cancels only the stream held by this API process; unknown id 404, other/completed
  stream 409. DELETE on a running capture remains 409.
- Oversize plans fail with /maxPackets before any VPP capture starts.
- Agent connection, Read and Delete repair interrupted records. Failed engine stop or
  filter restoration remains retryable; filter cleanup is bound to the original VPP boot.
- CaptureList is unpaged and bounded by configured retention plus a running capture.
- VPP packet-limit recording does not expose a count getter in the pinned binary API;
  finalization waits for timeout or Stop. No CLI polling or unsupported RPC was added.
