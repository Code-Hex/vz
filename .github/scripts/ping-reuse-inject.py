#!/usr/bin/env python3
"""Apply to the gofmt-formatted output of the 13a5191 ping-ring injector."""
from pathlib import Path
import sys

root = Path(sys.argv[1])

def replace(filename, old, new):
    path = root / filename
    contents = path.read_text()
    count = contents.count(old)
    if count != 1:
        raise SystemExit(f'{filename}: expected one match, got {count}')
    path.write_text(contents.replace(old, new, 1))

replace('network_device_integration_test.go', '\t"os"\n', '\t"os"\n\t"strconv"\n')
replace('network_device_integration_test.go', '\ta := newICMPPeer(t, netip.MustParseAddr("192.0.2.2")', '''\titerations := 60
\tif raw := os.Getenv("VZ_TRACE_RECONNECT_COUNT"); raw != "" {
\t\tvar err error
\t\titerations, err = strconv.Atoi(raw)
\t\tif err != nil || iterations < 1 || iterations > 80 {
\t\t\tt.Fatal("VZ_TRACE_RECONNECT_COUNT must be between 1 and 80")
\t\t}
\t}
\ta := newICMPPeer(t, netip.MustParseAddr("192.0.2.2")''')
replace('network_device_integration_test.go', 'echo CARRIER; cat /sys/class/net/eth1/carrier;', 'echo IFINDEX; cat /sys/class/net/eth1/ifindex; echo CARRIER; cat /sys/class/net/eth1/carrier;')
old = '''\ta.trace.phase.Store(2)
\tb.trace.phase.Store(2)
\tif err := device.SetAttachment(nil); err != nil {
\t\tt.Fatal(err)
\t}
\tping(b, false)
\ta.trace.phase.Store(3)
\tb.trace.phase.Store(3)
\tif err := device.SetAttachment(b.attachment); err != nil {
\t\tt.Fatal(err)
\t}
\tping(b, true)
'''
new = '''\tfor iteration := 1; iteration <= iterations; iteration++ {
\t\tfmt.Printf("VZTRACE reconnect iteration=%d/%d phase=nil\\n", iteration, iterations)
\t\ta.trace.phase.Store(uint32(iteration<<2 | 2))
\t\tb.trace.phase.Store(uint32(iteration<<2 | 2))
\t\tif err := device.SetAttachment(nil); err != nil {
\t\t\tt.Fatal(err)
\t\t}
\t\tping(b, false)
\t\tfmt.Printf("VZTRACE reconnect iteration=%d/%d phase=reattach\\n", iteration, iterations)
\t\ta.trace.phase.Store(uint32(iteration<<2 | 3))
\t\tb.trace.phase.Store(uint32(iteration<<2 | 3))
\t\tif err := device.SetAttachment(b.attachment); err != nil {
\t\t\tt.Fatal(err)
\t\t}
\t\tping(b, true)
\t}
'''
replace('network_device_integration_test.go', old, new)
replace('network_icmp_peer_test.go', 'seq=%d unix_ns=%d phase=%d dir=%c n=%d err=%v frame=%x", name, i, e.timeNS, e.phase, e.direction', 'seq=%d unix_ns=%d iteration=%d phase=%d dir=%c n=%d err=%v frame=%x", name, i, e.timeNS, e.phase>>2, e.phase&3, e.direction')
