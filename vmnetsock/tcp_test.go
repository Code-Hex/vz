package vmnetsock

import (
	"bytes"
	"encoding/binary"
	"net/netip"
	"sync"
	"testing"
	"time"
)

func TestTCPMSSConnections(t *testing.T) {
	now := time.Unix(100, 0)
	s := newTCPSegmentation(1500)
	for i, mss := range []uint16{256, 536, 1200} {
		peer := netip.AddrPortFrom(netip.MustParseAddr("192.0.2.2"), uint16(1000+i))
		remote := netip.MustParseAddrPort("192.0.2.1:80")
		syn := flowPacket(peer, remote, 2, 10, 0, mss, 0)
		if err := s.observePeer(syn, now); err != nil {
			t.Fatal(err)
		}
		ack := flowPacket(remote, peer, 0x12, 20, 11, 1460, 0)
		if err := s.forwardNative(ack, now, func([]byte) error { return nil }); err != nil {
			t.Fatal(err)
		}
		data := flowPacket(remote, peer, 0x18, 21, 11, 0, 2000)
		var sizes []int
		if err := s.forwardNative(data, now, func(frame []byte) error {
			sizes = append(sizes, len(frame)-54)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if len(sizes) < 2 || sizes[0] != int(mss) {
			t.Fatalf("MSS%d segments=%v", mss, sizes)
		}
	}
	if len(s.flows) != 3 {
		t.Fatalf("connections=%d", len(s.flows))
	}
}

func flowPacket(src, dst netip.AddrPort, flags byte, seq, ack uint32, mss uint16, payload int) []byte {
	th := 20
	if mss != 0 {
		th += 4
	}
	ih := 20
	if src.Addr().Is6() {
		ih = 40
	}
	b := make([]byte, 14+ih+th+payload)
	ip := b[14:]
	if ih == 20 {
		binary.BigEndian.PutUint16(b[12:], 0x800)
		ip[0] = 0x45
		ip[9] = 6
		binary.BigEndian.PutUint16(ip[2:], uint16(len(ip)))
		a, z := src.Addr().As4(), dst.Addr().As4()
		copy(ip[12:], a[:])
		copy(ip[16:], z[:])
	} else {
		binary.BigEndian.PutUint16(b[12:], 0x86dd)
		ip[0] = 0x60
		ip[6] = 6
		binary.BigEndian.PutUint16(ip[4:], uint16(th+payload))
		a, z := src.Addr().As16(), dst.Addr().As16()
		copy(ip[8:], a[:])
		copy(ip[24:], z[:])
	}
	tcp := ip[ih:]
	binary.BigEndian.PutUint16(tcp, src.Port())
	binary.BigEndian.PutUint16(tcp[2:], dst.Port())
	binary.BigEndian.PutUint32(tcp[4:], seq)
	binary.BigEndian.PutUint32(tcp[8:], ack)
	tcp[12] = byte(th/4) << 4
	tcp[13] = flags
	if mss != 0 {
		tcp[20] = 2
		tcp[21] = 4
		binary.BigEndian.PutUint16(tcp[22:], mss)
	}
	for i := th; i < len(tcp); i++ {
		tcp[i] = byte(i)
	}
	return b
}

func TestTCPHandshakeAndReuse(t *testing.T) {
	now := time.Unix(100, 0)
	for _, v6 := range []bool{false, true} {
		peer, remote := netip.MustParseAddrPort("192.0.2.2:1000"), netip.MustParseAddrPort("192.0.2.1:80")
		defaultMSS := 536
		if v6 {
			peer = netip.MustParseAddrPort("[2001:db8::2]:1000")
			remote = netip.MustParseAddrPort("[2001:db8::1]:80")
			defaultMSS = 1220
		}
		s := newTCPSegmentation(1500)
		sink := func([]byte) error { return nil }
		data := flowPacket(remote, peer, 0x18, 21, 11, 0, 100)
		if err := s.forwardNative(data, now, sink); err == nil {
			t.Fatal("unobserved connection accepted data below MTU")
		}
		if err := s.forwardNative(flowPacket(remote, peer, 2, 20, 0, 1460, 0), now, sink); err != nil {
			t.Fatal(err)
		}
		if err := s.observePeer(flowPacket(peer, remote, 0x12, 10, 21, 0, 0), now); err != nil {
			t.Fatal(err)
		}
		var first int
		if err := s.forwardNative(flowPacket(remote, peer, 0x18, 21, 11, 0, 3000), now, func(frame []byte) error {
			p, e := parseTCPPacket(frame)
			if e != nil {
				return e
			}
			if first == 0 {
				first = p.payload()
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if first != defaultMSS {
			t.Fatalf("SYN without option used %d, want%d", first, defaultMSS)
		}
		// A new ISN invalidates old MSS knowledge until its matching SYN-ACK.
		if err := s.observePeer(flowPacket(peer, remote, 2, 100, 0, 256, 0), now); err != nil {
			t.Fatal(err)
		}
		if err := s.forwardNative(data, now, sink); err == nil {
			t.Fatal("pending reused connection retained old MSS")
		}
		if err := s.forwardNative(flowPacket(remote, peer, 0x12, 20, 11, 1460, 0), now, sink); err == nil {
			t.Fatal("stale SYN-ACK accepted")
		}
		if err := s.forwardNative(flowPacket(remote, peer, 0x12, 200, 101, 1460, 0), now, sink); err != nil {
			t.Fatal(err)
		}
		if err := s.observePeer(flowPacket(peer, remote, 2, 100, 0, 256, 0), now); err != nil {
			t.Fatal(err)
		}
		if err := s.forwardNative(flowPacket(remote, peer, 0x18, 201, 101, 0, 600), now, sink); err != nil {
			t.Fatal(err)
		}
		if err := s.observePeer(flowPacket(peer, remote, 2, 100, 0, 1200, 0), now); err == nil {
			t.Fatal("changed retransmitted MSS accepted")
		}
	}
}

func TestTCPFlowRelease(t *testing.T) {
	peer, remote := netip.MustParseAddrPort("192.0.2.2:1000"), netip.MustParseAddrPort("192.0.2.1:80")
	now := time.Unix(100, 0)
	s := newTCPSegmentation(1500)
	sink := func([]byte) error { return nil }
	if err := s.observePeer(flowPacket(peer, remote, 2, 10, 0, 536, 0), now); err != nil {
		t.Fatal(err)
	}
	if err := s.forwardNative(flowPacket(remote, peer, 0x12, 20, 11, 1460, 0), now, sink); err != nil {
		t.Fatal(err)
	}
	if err := s.observePeer(flowPacket(peer, remote, 0x11, 11, 21, 0, 0), now); err != nil {
		t.Fatal(err)
	}
	now = now.Add(3 * time.Minute)
	if err := s.forwardNative(flowPacket(remote, peer, 0x18, 21, 12, 0, 1000), now, sink); err != nil {
		t.Fatalf("half-close discarded MSS: %v", err)
	}
	if err := s.forwardNative(flowPacket(remote, peer, 0x11, 1021, 12, 0, 0), now, sink); err != nil {
		t.Fatal(err)
	}
	if err := s.observePeer(flowPacket(peer, remote, 0x10, 12, 1022, 0, 0), now); err != nil {
		t.Fatal(err)
	}
	if err := s.forwardNative(flowPacket(remote, peer, 0x19, 21, 12, 0, 1000), now, sink); err != nil {
		t.Fatalf("FIN retransmission discarded MSS: %v", err)
	}
	if err := s.forwardNative(flowPacket(remote, peer, 0x18, 21, 12, 0, 100), now.Add(tcpClosedTimeout), sink); err == nil {
		t.Fatal("expired closed connection accepted data")
	}
	if len(s.flows) != 0 {
		t.Fatal("closed connection retained after expiry")
	}
	if err := s.observePeer(flowPacket(peer, remote, 2, 100, 0, 256, 0), now); err != nil {
		t.Fatal(err)
	}
	if err := s.forwardNative(flowPacket(remote, peer, 4, 0, 101, 0, 0), now, sink); err != nil {
		t.Fatal(err)
	}
	if len(s.flows) != 1 {
		t.Fatal("reset discarded connection before grace period")
	}
	if err := s.observePeer(flowPacket(peer, remote, 0x10, 101, 0, 0, 0), now.Add(tcpClosedTimeout)); err != nil {
		t.Fatal(err)
	}
	if len(s.flows) != 0 {
		t.Fatal("reset retained connection after expiry")
	}
}

func TestTCPResetGrace(t *testing.T) {
	now := time.Unix(100, 0)
	s := newTCPSegmentation(1500)
	remote := netip.MustParseAddrPort("192.0.2.1:80")
	peers := []netip.AddrPort{
		netip.MustParseAddrPort("192.0.2.2:1000"),
		netip.MustParseAddrPort("192.0.2.2:1001"),
	}
	sink := func([]byte) error { return nil }
	for _, peer := range peers {
		if err := s.observePeer(flowPacket(peer, remote, 2, 10, 0, 256, 0), now); err != nil {
			t.Fatal(err)
		}
		if err := s.forwardNative(flowPacket(remote, peer, 0x12, 20, 11, 1460, 0), now, sink); err != nil {
			t.Fatal(err)
		}
	}
	reset := flowPacket(peers[0], remote, 0x14, 11, 21, 0, 0)
	for _, when := range []time.Time{now, now.Add(time.Minute)} {
		if err := s.observePeer(reset, when); err != nil {
			t.Fatal(err)
		}
	}
	for _, peer := range peers {
		if err := s.forwardNative(flowPacket(remote, peer, 0x18, 21, 11, 0, 1000), now.Add(time.Minute), func(frame []byte) error {
			if len(frame)-54 > 256 {
				t.Fatal("reset lost the observed MSS")
			}
			return nil
		}); err != nil {
			t.Fatalf("data after reset: %v", err)
		}
	}
	if len(s.flows) != 2 {
		t.Fatal("reset changed the number of retained connections")
	}
	if err := s.forwardNative(flowPacket(remote, peers[0], 0x18, 21, 11, 0, 100), now.Add(tcpClosedTimeout), sink); err == nil {
		t.Fatal("repeated reset extended the grace period")
	}
	if len(s.flows) != 1 {
		t.Fatal("reset entry did not expire")
	}
	now = now.Add(tcpClosedTimeout)
	peer := peers[1]
	if err := s.observePeer(flowPacket(peer, remote, 0x14, 11, 21, 0, 0), now); err != nil {
		t.Fatal(err)
	}
	if err := s.observePeer(flowPacket(peer, remote, 2, 10, 0, 536, 0), now); err != nil {
		t.Fatalf("new SYN with reused ISN: %v", err)
	}
	if err := s.forwardNative(flowPacket(remote, peer, 0x18, 21, 11, 0, 100), now, sink); err == nil {
		t.Fatal("new SYN reused the reset connection's handshake")
	}
	if err := s.forwardNative(flowPacket(remote, peer, 0x12, 200, 11, 1460, 0), now, sink); err != nil {
		t.Fatal(err)
	}
	if err := s.forwardNative(flowPacket(remote, peer, 0x18, 201, 11, 0, 100), now.Add(tcpClosedTimeout), sink); err != nil {
		t.Fatalf("new handshake retained reset expiry: %v", err)
	}
}

func TestTCPFlowBoundAndIdleExpiry(t *testing.T) {
	s := newTCPSegmentation(1500)
	now := time.Unix(100, 0)
	remote := netip.MustParseAddrPort("192.0.2.1:80")
	for i := 0; i < maxTCPFlows; i++ {
		peer := netip.AddrPortFrom(netip.MustParseAddr("192.0.2.2"), uint16(i+1000))
		if err := s.observePeer(flowPacket(peer, remote, 2, 10, 0, 536, 0), now); err != nil {
			t.Fatal(err)
		}
	}
	extra := netip.MustParseAddrPort("192.0.2.2:60000")
	if err := s.observePeer(flowPacket(extra, remote, 2, 10, 0, 536, 0), now); err == nil {
		t.Fatal("unbounded flow admission")
	}
	if err := s.observePeer(flowPacket(extra, remote, 2, 10, 0, 536, 0), now.Add(tcpHandshakeTimeout)); err != nil {
		t.Fatal(err)
	}
	if len(s.flows) != 1 {
		t.Fatalf("expired handshakes retained: %d", len(s.flows))
	}
	sink := func([]byte) error { return nil }
	later := now.Add(tcpHandshakeTimeout)
	if err := s.forwardNative(flowPacket(remote, extra, 0x12, 20, 11, 1460, 0), later, sink); err != nil {
		t.Fatal(err)
	}
	if err := s.forwardNative(flowPacket(remote, extra, 0x18, 21, 11, 0, 1), later.Add(tcpIdleTimeout), sink); err == nil {
		t.Fatal("expired idle connection accepted data")
	}
	if len(s.flows) != 0 {
		t.Fatal("idle connection retained after expiry")
	}
}

func TestTCPSegmentationOutput(t *testing.T) {
	for _, v6 := range []bool{false, true} {
		src, dst := netip.MustParseAddrPort("192.0.2.1:80"), netip.MustParseAddrPort("192.0.2.2:1000")
		if v6 {
			src = netip.MustParseAddrPort("[2001:db8::1]:80")
			dst = netip.MustParseAddrPort("[2001:db8::2]:1000")
		}
		original := flowPacket(src, dst, 0x99, 0xfffffff0, 1, 0, 2000)
		p, err := parseTCPPacket(original)
		if err != nil {
			t.Fatal(err)
		}
		binary.BigEndian.PutUint16(p.ip[4:], 65535)
		if v6 {
			binary.BigEndian.PutUint16(p.ip[4:], uint16(len(p.ip)-40))
		}
		var joined []byte
		segments := 0
		err = p.emit(1500, 536, func(frame []byte) error {
			segment, e := parseTCPPacket(frame)
			if e != nil {
				return e
			}
			if segment.payload() > 536 || len(segment.ip) > 1500 {
				t.Fatal("segment exceeds MSS or MTU")
			}
			if got := binary.BigEndian.Uint32(segment.tcp[4:]); got != uint32(0xfffffff0)+uint32(len(joined)) {
				t.Fatalf("sequence=%d", got)
			}
			last := len(joined)+segment.payload() == 2000
			wantFlags := byte(0x10)
			if segments == 0 {
				wantFlags |= 0x80
			}
			if last {
				wantFlags |= 9
			}
			if segment.tcp[13] != wantFlags {
				t.Fatalf("flags=%x, want%x", segment.tcp[13], wantFlags)
			}
			pseudo := make([]byte, 0, 40)
			if v6 {
				pseudo = append(pseudo, segment.ip[8:40]...)
				pseudo = append(pseudo, 0, 0, byte(len(segment.tcp)>>8), byte(len(segment.tcp)), 0, 0, 0, 6)
			} else {
				if binary.BigEndian.Uint16(segment.ip[4:]) != uint16(65535+segments) || wireSum(segment.ip[:20]) != 65535 {
					t.Fatal("IPv4 ID or checksum")
				}
				pseudo = append(pseudo, segment.ip[12:20]...)
				pseudo = append(pseudo, 0, 6, byte(len(segment.tcp)>>8), byte(len(segment.tcp)))
			}
			if wireSum(append(pseudo, segment.tcp...)) != 65535 {
				t.Fatal("TCP checksum")
			}
			joined = append(joined, segment.tcp[segment.tcpHeader:]...)
			segments++
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if segments != 4 || !bytes.Equal(joined, p.tcp[p.tcpHeader:]) {
			t.Fatal("segmentation changed payload")
		}
	}
}

func wireSum(b []byte) uint32 {
	var sum uint32
	for i, v := range b {
		if i%2 == 0 {
			sum += uint32(v) << 8
		} else {
			sum += uint32(v)
		}
	}
	for sum > 65535 {
		sum = (sum & 65535) + (sum >> 16)
	}
	return sum
}

func TestTCPConcurrentDirections(t *testing.T) {
	peer, remote := netip.MustParseAddrPort("192.0.2.2:1000"), netip.MustParseAddrPort("192.0.2.1:80")
	now := time.Unix(100, 0)
	s := newTCPSegmentation(1500)
	sink := func([]byte) error { return nil }
	if err := s.observePeer(flowPacket(peer, remote, 2, 10, 0, 536, 0), now); err != nil {
		t.Fatal(err)
	}
	if err := s.forwardNative(flowPacket(remote, peer, 0x12, 20, 11, 1460, 0), now, sink); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			if err := s.observePeer(flowPacket(peer, remote, 0x10, 11, 21, 0, 0), now); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			if err := s.forwardNative(flowPacket(remote, peer, 0x18, 21, 11, 0, 2000), now, sink); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	wg.Wait()
	s.clear()
	if len(s.flows) != 0 {
		t.Fatal("completed pumps retained connection info")
	}
}

func TestTCPPacketBoundaries(t *testing.T) {
	src, dst := netip.MustParseAddrPort("192.0.2.1:80"), netip.MustParseAddrPort("192.0.2.2:1000")
	valid := flowPacket(src, dst, 0x10, 1, 1, 0, 0)
	padded := append(bytes.Clone(valid), make([]byte, 2000)...)
	p, err := parseTCPPacket(padded)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.emit(1500, 0, func(out []byte) error {
		if !bytes.Equal(out, valid) {
			t.Fatal("IP padding escaped parser")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []struct {
		name   string
		packet []byte
	}{
		{"short", []byte{1}},
		{"short IP", valid[:30]},
		{"short TCP", valid[:len(valid)-1]},
	} {
		t.Run(bad.name, func(t *testing.T) {
			if _, err := parseTCPPacket(bad.packet); err == nil {
				t.Fatal("malformed packet accepted")
			}
		})
	}
	syn := flowPacket(dst, src, 2, 10, 0, 536, 0)
	for _, length := range []byte{0, 1, 3, 5} {
		bad := bytes.Clone(syn)
		bad[14+20+21] = length
		if _, err := parseTCPPacket(bad); err == nil {
			t.Fatalf("MSS length %d accepted", length)
		}
	}
	zero := bytes.Clone(syn)
	zero[len(zero)-2], zero[len(zero)-1] = 0, 0
	if _, err := parseTCPPacket(zero); err == nil {
		t.Fatal("zero MSS accepted")
	}
	// MLD uses Hop-by-Hop options but is not TCP.
	mld := make([]byte, 14+40+8+24)
	binary.BigEndian.PutUint16(mld[12:], 0x86dd)
	mld[14] = 0x60
	mld[20] = 0
	binary.BigEndian.PutUint16(mld[18:], 32)
	mld[54] = 58
	if p, err := parseTCPPacket(mld); err != nil || p.tcp != nil {
		t.Fatalf("non-TCP IPv6 extension: %v, %v", p, err)
	}
	mld[54] = 6
	if _, err := parseTCPPacket(mld); err == nil {
		t.Fatal("TCP IPv6 extension accepted")
	}
}

func TestTCPSegmentationUnsupportedOptions(t *testing.T) {
	peer := netip.MustParseAddrPort("192.0.2.2:1000")
	remote := netip.MustParseAddrPort("192.0.2.1:80")
	for _, kind := range []byte{19, 29, 30, 253} {
		frame := flowPacket(remote, peer, 0x18, 1, 1, 536, 100)
		frame[54] = kind
		packet, err := parseTCPPacket(frame)
		if err != nil {
			t.Fatal(err)
		}
		emitted := 0
		emit := func([]byte) error {
			emitted++
			return nil
		}
		if err := packet.emit(1500, 536, emit); err != nil || emitted != 1 {
			t.Fatalf("option %d unchanged frame: emitted=%d, error=%v", kind, emitted, err)
		}
		if err := packet.emit(1500, 64, emit); err == nil || emitted != 1 {
			t.Fatalf("option %d split frame: emitted=%d, error=%v", kind, emitted, err)
		}
	}
}
