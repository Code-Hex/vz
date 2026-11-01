from pathlib import Path
import sys
p = Path(sys.argv[1]) if len(sys.argv) > 1 else Path('source')
def edit(name, old, new):
    f = p/name
    s = f.read_text()
    n = s.count(old)
    if n != 1 and not (name == 'network_device_integration_test.go' and (old.startswith('\tvm := newVirtualizationMachine') or old == '\tdevice := devices[1]\n' or old.startswith('\tif err := device.SetAttachment(b.attachment)') or old.startswith('\tif err := device.SetAttachment(nil)')) and n == 2):
        raise SystemExit(f'{name}: expected one match, got {n}: {old!r}')
    f.write_text(s.replace(old, new, 1))
edit('virtualization_test.go', '\tsocketDevices := vm.SocketDevices()', '\tif networkTraceBeforeStart != nil { networkTraceBeforeStart(vm) }\n\tsocketDevices := vm.SocketDevices()')
edit('network_icmp_peer_test.go', '\t"testing"', '\t"testing"\n\t"sync"\n\t"sync/atomic"')
edit('network_icmp_peer_test.go', 'type icmpPeer struct {', '''type packetEvent struct {
 timeNS int64
 phase uint32
 direction byte
 n int
 captured int
 err error
 data [98]byte
}
type packetRing struct {
 mu sync.Mutex
 phase atomic.Uint32
 count uint64
 events [256]packetEvent
}
func (r *packetRing) add(direction byte, n int, err error, data []byte) {
 e := packetEvent{timeNS: time.Now().UnixNano(), phase: r.phase.Load(), direction: direction, n: n, err: err}
 e.captured = copy(e.data[:], data)
 r.mu.Lock()
 r.events[r.count%uint64(len(r.events))] = e
 r.count++
 r.mu.Unlock()
}
func (r *packetRing) dump(t *testing.T, name string) {
 r.mu.Lock()
 defer r.mu.Unlock()
 start := uint64(0)
 if r.count > uint64(len(r.events)) { start = r.count - uint64(len(r.events)) }
 t.Logf("TRACE peer=%s events=%d retained=%d", name, r.count, r.count-start)
 for i := start; i < r.count; i++ {
  e := r.events[i%uint64(len(r.events))]
  size := e.captured
  t.Logf("TRACE peer=%s seq=%d unix_ns=%d phase=%d dir=%c n=%d err=%v frame=%x", name, i, e.timeNS, e.phase, e.direction, e.n, e.err, e.data[:size])
 }
}

type icmpPeer struct {''')
edit('network_icmp_peer_test.go', '\tattachment *vz.FileHandleNetworkDeviceAttachment', '\ttrace packetRing\n\tattachment *vz.FileHandleNetworkDeviceAttachment')
edit('network_icmp_peer_test.go', 'serveICMP(conn, ip, mac)', 'serveICMP(conn, ip, mac, &peer.trace)')
edit('network_icmp_peer_test.go', 'func serveICMP(conn net.Conn, ip netip.Addr, mac [6]byte) error {', 'func serveICMP(conn net.Conn, ip netip.Addr, mac [6]byte, trace *packetRing) error {')
edit('network_icmp_peer_test.go', '\t\tn, err := conn.Read(buffer)\n', '\t\tn, err := conn.Read(buffer)\n\t\ttrace.add(\'R\', n, err, buffer[:max(n, 0)])\n')
edit('network_icmp_peer_test.go', '\t\tif err := conn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {\n\t\t\treturn err\n\t\t}\n\t\twritten, err := conn.Write(reply)', '\t\tif err := conn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {\n\t\t\ttrace.add(\'D\', 0, err, nil)\n\t\t\treturn err\n\t\t}\n\t\twritten, err := conn.Write(reply)\n\t\ttrace.add(\'W\', written, err, reply)')
edit('network_device_integration_test.go', 'func TestRuntimeNetworkConnection(t *testing.T) {', 'var networkTraceBeforeStart func(*vz.VirtualMachine)\n\nfunc TestRuntimeNetworkConnection(t *testing.T) {')
edit('network_device_integration_test.go', '\tvm := newVirtualizationMachine(t, func(config *vz.VirtualMachineConfiguration) error {', '''
 events := make(chan *vz.DisconnectedError, 256)
 networkTraceBeforeStart = func(machine *vz.VirtualMachine) {
  stop, err := machine.NetworkDeviceAttachmentWasDisconnectedNotify(events)
  if err != nil { t.Fatalf("trace subscribe: %v", err) }
  t.Cleanup(stop)
 }
 defer func() { networkTraceBeforeStart = nil }()
 vm := newVirtualizationMachine(t, func(config *vz.VirtualMachineConfiguration) error {''')
edit('network_device_integration_test.go', '\trun := func(command string) ([]byte, error) {', '''
 var device *vz.NetworkDevice
 t.Cleanup(func() {
  if !t.Failed() { return }
  a.trace.dump(t, "A")
  b.trace.dump(t, "B")
  if device != nil { current := device.Attachment(); t.Logf("TRACE attachment nil=%t type=%T", current == nil, current) }
  t.Logf("TRACE disconnected buffered=%d capacity=%d", len(events), cap(events))
  for { select { case event := <-events: t.Logf("TRACE disconnected=%+v", event); default: return } }
 })
 run := func(command string) ([]byte, error) {''')
edit('network_device_integration_test.go', '\t\t\t\tt.Fatalf("ping %s: %v\\n%s", peer.ip, err, output)', '''
                // Snapshot after failure only, preserve original ping result even if SSH is broken.
                if deadlineErr := vm.conn.SetDeadline(time.Now().Add(15*time.Second)); deadlineErr != nil {
                    t.Logf("TRACE diagnostic deadline error=%v", deadlineErr)
                } else {
                    session, sessionErr := vm.Client.NewSession()
                    if sessionErr != nil { t.Logf("TRACE diagnostic session error=%v", sessionErr) } else {
                        state, stateErr := session.CombinedOutput("echo CARRIER; cat /sys/class/net/eth1/carrier; echo LINK; ip link show dev eth1; echo ADDR; ip addr show dev eth1; echo ARP; arp -n -i eth1; echo PROC_ARP; cat /proc/net/arp; echo ROUTES; ip route; echo STATS; ip -s link show dev eth1; echo SYS_STATS; for f in /sys/class/net/eth1/statistics/*; do echo $f; cat $f; done; echo DMESG; dmesg | tail -80")
                        t.Logf("TRACE guest diagnostic err=%v output=\\n%s", stateErr, state)
                        session.Close()
                    }
                    vm.conn.SetDeadline(time.Time{})
                }
                t.Fatalf("ping %s: %v\\n%s", peer.ip, err, output)''')
edit('network_device_integration_test.go', '\tdevice := devices[1]\n', '\tdevice = devices[1]\n')
edit('network_device_integration_test.go', '\tif err := device.SetAttachment(b.attachment); err != nil {', '\ta.trace.phase.Store(1); b.trace.phase.Store(1)\n\tif err := device.SetAttachment(b.attachment); err != nil {')
edit('network_device_integration_test.go', '\tif err := device.SetAttachment(nil); err != nil {', '\ta.trace.phase.Store(2); b.trace.phase.Store(2)\n\tif err := device.SetAttachment(nil); err != nil {')
old='\tping(b, false)\n\tif err := device.SetAttachment(b.attachment); err != nil {'
edit('network_device_integration_test.go', old, '\tping(b, false)\n\ta.trace.phase.Store(3); b.trace.phase.Store(3)\n\tif err := device.SetAttachment(b.attachment); err != nil {')
