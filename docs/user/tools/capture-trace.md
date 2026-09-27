# Packet capture (Tools → Packet capture)

Capture packets on a VPP interface and download a `.pcap` file you can open in Wireshark or with `tcpdump -r`.

## Start a capture
1. Open **Tools → Packet capture**, tab **Capture**.
2. **Interface**: the VPP interface name (for example `loop501`, `GigabitEthernet0/8/0`) or `any`. Only interfaces this
   system owns can be captured.
3. **Direction**: receive, transmit or both. Tick **Also capture dropped packets** to add VPP's drop capture.
4. **BPF filter** (optional): a pcap-filter expression such as `icmp and host 10.0.0.1`. Quotes, `;`, `$` and
   backslashes are refused (400 with the field highlighted). The filter needs the globals-owner agent (the product
   agent on a real box); otherwise the start answers 409.
5. **Limits**: max packets (1–100000, default 1000), seconds (1–600, default 30), bytes per packet (32–9000).
6. **Start capture**. A progress bar shows the running capture. The capture stops after the time limit (VPP stops
   recording by itself at the packet limit).

Only one capture runs per VPP. A second start answers **409 `capture-busy`**.

## Files
VPP writes the file under `/tmp`, readable by everyone. The agent moves it to `/var/lib/vrx/captures/` with mode 0600
and records its size, packet count and sha256. It keeps the 10 newest files and at most 500 MB
(`VRX_CAPTURE_MAX_FILES`, `VRX_CAPTURE_MAX_BYTES`). Older files are deleted first.
**Download** and **Delete** are for administrators only. Every download is written to the audit log.

```
tcpdump -nr vrx-20260927T100000-1.pcap
```

If the agent restarts during a capture, the capture is stopped the next time the capture list is read. It is then
shown as **Interrupted**, and the file is kept if VPP wrote one.

## Trace and packet generator
These tabs say **Not available on this build**:
- Packet trace is banned on the shared VPP because `show trace` can crash VPP (D-128/TD-20). The tracedump/tracenode
  plugins and their binary API are also not built yet (V18).
- The packet generator has no binary API for defining streams (only `cli_inband`, which is not allowed).
