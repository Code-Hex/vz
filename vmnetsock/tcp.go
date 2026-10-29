package vmnetsock

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"sync"
	"time"
)

const (
	// These are adapter memory policies, not vmnet or TCP protocol limits.
	// Keep at most 4096 handshakes and fail admission instead of forgetting an
	// active connection's MSS. Give idle connections a longer lifetime than
	// unfinished handshakes and completed closes, which only need retry time.
	maxTCPFlows         = 4096
	tcpIdleTimeout      = 30 * time.Minute
	tcpHandshakeTimeout = 2 * time.Minute
	tcpClosedTimeout    = 2 * time.Minute
)

type tcpFlowKey struct {
	peer   netip.AddrPort
	remote netip.AddrPort
}

type tcpFlowPhase uint8

const (
	tcpPeerSYN tcpFlowPhase = iota
	tcpNativeSYN
	tcpReady
)

type tcpFlow struct {
	phase              tcpFlowPhase
	peerISN, nativeISN uint32
	mss                int
	lastSeen, closedAt time.Time
	fin                [2]uint32
	finSeen, finAcked  uint8
}

func (f tcpFlow) expired(now time.Time) bool {
	if !f.closedAt.IsZero() {
		return now.Sub(f.closedAt) >= tcpClosedTimeout
	}
	timeout := tcpIdleTimeout
	if f.phase != tcpReady {
		timeout = tcpHandshakeTimeout
	}
	return now.Sub(f.lastSeen) >= timeout
}

type tcpSegmentation struct {
	mu    sync.Mutex
	mtu   int
	flows map[tcpFlowKey]tcpFlow
}

func newTCPSegmentation(mtu int) *tcpSegmentation {
	return &tcpSegmentation{mtu: mtu, flows: make(map[tcpFlowKey]tcpFlow)}
}

func (s *tcpSegmentation) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flows = nil
}

func (s *tcpSegmentation) observePeer(frame []byte, now time.Time) error {
	packet, err := parseTCPPacket(frame)
	if err != nil {
		return err
	}
	if packet == nil {
		return nil
	}
	_, err = s.observe(packet, true, now)
	return err
}

func (s *tcpSegmentation) forwardNative(frame []byte, now time.Time, emit func([]byte) error) error {
	packet, err := parseTCPPacket(frame)
	if err != nil {
		return err
	}
	if packet == nil {
		if len(frame) > s.mtu+14 {
			return fmt.Errorf("cannot segment oversized non-TCP frame")
		}
		return emit(frame)
	}
	mss, err := s.observe(packet, false, now)
	if err != nil {
		return err
	}
	return packet.emit(s.mtu, mss, emit)
}

func (s *tcpSegmentation) observe(p *tcpPacket, peer bool, now time.Time) (int, error) {
	key := tcpFlowKey{peer: p.src, remote: p.dst}
	direction := 0
	if !peer {
		key.peer, key.remote = key.remote, key.peer
		direction = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, known := s.flows[key]
	if known && f.expired(now) {
		delete(s.flows, key)
		known = false
	}
	flags := p.tcp[13]
	if flags&4 != 0 {
		if p.payload() != 0 {
			return 0, fmt.Errorf("TCP reset contains data")
		}
		// The opposite pump can still receive data sent before the reset.
		if known && f.closedAt.IsZero() {
			f.closedAt = now
			s.flows[key] = f
		}
		return 0, nil
	}
	seq, ack := binary.BigEndian.Uint32(p.tcp[4:]), binary.BigEndian.Uint32(p.tcp[8:])
	if flags&2 != 0 {
		if !known && flags&0x10 == 0 {
			for key, entry := range s.flows {
				if entry.expired(now) {
					delete(s.flows, key)
				}
			}
			if len(s.flows) >= maxTCPFlows {
				return 0, fmt.Errorf("TCP MSS table is full (%d connections)", maxTCPFlows)
			}
		}
		var err error
		f, known, err = f.observeSYN(p, peer, known)
		if err != nil {
			return 0, err
		}
	}
	if !peer && p.payload() != 0 && (!known || f.phase != tcpReady) {
		return 0, fmt.Errorf("TCP data has no observed MSS handshake")
	}
	if !known {
		return 0, nil
	}
	f.lastSeen = now
	opposite := 1 - direction
	if flags&0x10 != 0 && f.finSeen&(1<<opposite) != 0 && int32(ack-f.fin[opposite]) >= 0 {
		f.finAcked |= 1 << opposite
	}
	if flags&1 != 0 {
		end := seq + uint32(p.payload()) + 1
		if f.finSeen&(1<<direction) != 0 && f.fin[direction] != end {
			return 0, fmt.Errorf("TCP FIN retransmission changed sequence")
		}
		f.fin[direction] = end
		f.finSeen |= 1 << direction
	}
	// Keep the MSS for FIN/data retransmission after both half-closes finish.
	if f.finAcked == 3 && f.closedAt.IsZero() {
		f.closedAt = now
	}
	s.flows[key] = f
	return f.mss, nil
}

func (f tcpFlow) observeSYN(p *tcpPacket, peer, known bool) (tcpFlow, bool, error) {
	if p.payload() != 0 {
		return f, known, fmt.Errorf("TCP SYN data is not supported for segmentation")
	}
	seq := binary.BigEndian.Uint32(p.tcp[4:])
	ack := binary.BigEndian.Uint32(p.tcp[8:])
	if p.tcp[13]&0x10 == 0 {
		expectedISN := f.peerISN
		sameDirection := f.phase != tcpNativeSYN
		if !peer {
			expectedISN = f.nativeISN
			sameDirection = f.phase != tcpPeerSYN
		}
		if !known || !f.closedAt.IsZero() || !sameDirection || expectedISN != seq {
			f = tcpFlow{phase: tcpNativeSYN, nativeISN: seq}
			if peer {
				f.phase = tcpPeerSYN
				f.peerISN = seq
				f.mss = p.mss
			}
		} else if peer && f.mss != p.mss {
			return f, known, fmt.Errorf("TCP SYN retransmission changed MSS")
		}
		return f, true, nil
	}
	if !known {
		return f, false, nil
	}
	if peer {
		if f.phase == tcpPeerSYN || ack != f.nativeISN+1 {
			return f, known, fmt.Errorf("TCP SYN-ACK does not match native SYN")
		}
		if f.phase == tcpReady && (f.peerISN != seq || f.mss != p.mss) {
			return f, known, fmt.Errorf("TCP SYN-ACK retransmission changed peer parameters")
		}
		f.peerISN = seq
		f.mss = p.mss
	} else {
		if f.phase == tcpNativeSYN || ack != f.peerISN+1 {
			return f, known, fmt.Errorf("TCP SYN-ACK does not match peer SYN")
		}
		if f.phase == tcpReady && f.nativeISN != seq {
			return f, known, fmt.Errorf("TCP SYN-ACK retransmission changed native sequence")
		}
		f.nativeISN = seq
	}
	f.phase = tcpReady
	return f, true, nil
}

type tcpPacket struct {
	frame, ip, tcp                    []byte
	src, dst                          netip.AddrPort
	version, ipHeader, tcpHeader, mss int
	unsupportedOption                 bool
}

func parseTCPPacket(frame []byte) (*tcpPacket, error) {
	if len(frame) < 14 {
		return nil, fmt.Errorf("short Ethernet frame")
	}
	p := &tcpPacket{frame: frame, ip: frame[14:]}
	var src, dst netip.Addr
	switch binary.BigEndian.Uint16(frame[12:]) {
	case 0x8100, 0x88a8:
		return nil, fmt.Errorf("VLAN frames are not supported with TCP segmentation")
	case 0x800:
		if len(p.ip) < 20 || p.ip[0]>>4 != 4 {
			return nil, fmt.Errorf("invalid IPv4 header")
		}
		p.version = 4
		p.ipHeader = int(p.ip[0]&15) * 4
		n := int(binary.BigEndian.Uint16(p.ip[2:]))
		if p.ipHeader < 20 || n < p.ipHeader || n > len(p.ip) {
			return nil, fmt.Errorf("invalid IPv4 length")
		}
		p.ip = p.ip[:n]
		if p.ip[9] != 6 {
			return nil, nil
		}
		if p.ipHeader != 20 || binary.BigEndian.Uint16(p.ip[6:])&0x3fff != 0 {
			return nil, fmt.Errorf("TCP IPv4 options and fragments are not supported")
		}
		src = netip.AddrFrom4([4]byte(p.ip[12:16]))
		dst = netip.AddrFrom4([4]byte(p.ip[16:20]))
	case 0x86dd:
		if len(p.ip) < 40 || p.ip[0]>>4 != 6 {
			return nil, fmt.Errorf("invalid IPv6 header")
		}
		p.version = 6
		p.ipHeader = 40
		n := 40 + int(binary.BigEndian.Uint16(p.ip[4:]))
		if n > len(p.ip) {
			return nil, fmt.Errorf("invalid IPv6 length")
		}
		p.ip = p.ip[:n]
		isTCP, err := ipv6TCPHeader(p.ip)
		if err != nil || !isTCP {
			return nil, err
		}
		src = netip.AddrFrom16([16]byte(p.ip[8:24]))
		dst = netip.AddrFrom16([16]byte(p.ip[24:40]))
	default:
		return nil, nil
	}
	p.frame = frame[:14+len(p.ip)]
	p.tcp = p.ip[p.ipHeader:]
	if len(p.tcp) < 20 {
		return nil, fmt.Errorf("short TCP header")
	}
	p.tcpHeader = int(p.tcp[12]>>4) * 4
	if p.tcpHeader < 20 || p.tcpHeader > len(p.tcp) {
		return nil, fmt.Errorf("invalid TCP header length")
	}
	p.src = netip.AddrPortFrom(src, binary.BigEndian.Uint16(p.tcp))
	p.dst = netip.AddrPortFrom(dst, binary.BigEndian.Uint16(p.tcp[2:]))
	if err := p.parseOptions(); err != nil {
		return nil, err
	}
	return p, nil
}

func ipv6TCPHeader(ip []byte) (bool, error) {
	next := ip[6]
	offset := 40
	for next == 0 || next == 43 || next == 60 || next == 51 || next == 44 {
		if next == 44 {
			return false, fmt.Errorf("IPv6 fragments are not supported with TCP segmentation")
		}
		if offset+2 > len(ip) {
			return false, fmt.Errorf("short IPv6 extension")
		}
		length := (int(ip[offset+1]) + 1) * 8
		if next == 51 {
			length = (int(ip[offset+1]) + 2) * 4
		}
		if offset+length > len(ip) {
			return false, fmt.Errorf("invalid IPv6 extension length")
		}
		next = ip[offset]
		offset += length
	}
	if next != 6 {
		return false, nil
	}
	if offset != 40 {
		return false, fmt.Errorf("TCP IPv6 extensions are not supported")
	}
	return true, nil
}

func (p *tcpPacket) parseOptions() error {
	// RFC 9293 section 3.7.1 supplies these defaults only when an observed
	// SYN omits MSS. A connection whose SYN was missed never reaches tcpReady.
	// https://www.rfc-editor.org/rfc/rfc9293.html#section-3.7.1
	p.mss = 536
	if p.version == 6 {
		p.mss = 1220
	}
	seenMSS := false
	for options := p.tcp[20:p.tcpHeader]; len(options) > 0; {
		kind := options[0]
		if kind == 0 {
			break
		}
		if kind == 1 {
			options = options[1:]
			continue
		}
		if len(options) < 2 || options[1] < 2 || int(options[1]) > len(options) {
			return fmt.Errorf("invalid TCP option length")
		}
		switch kind {
		case 2, 3, 4, 5, 8:
			// MSS, window scale, SACK, and timestamps survive segmentation.
		default:
			p.unsupportedOption = true
		}
		if kind == 2 {
			if options[1] != 4 || seenMSS {
				return fmt.Errorf("invalid TCP MSS option")
			}
			p.mss = int(binary.BigEndian.Uint16(options[2:]))
			seenMSS = true
			if p.mss == 0 {
				return fmt.Errorf("TCP MSS is zero")
			}
		}
		options = options[int(options[1]):]
	}
	return nil
}

func (p *tcpPacket) payload() int {
	return len(p.tcp) - p.tcpHeader
}

func (p *tcpPacket) emit(mtu, mss int, emit func([]byte) error) error {
	if p.payload() == 0 {
		if len(p.ip) > mtu {
			return fmt.Errorf("TCP headers exceed MTU")
		}
		return emit(p.frame)
	}
	// MSS excludes fixed headers. Options consume part of its data allowance.
	// https://www.rfc-editor.org/rfc/rfc9293.html#section-3.7.1
	limit := min(mtu-p.ipHeader-p.tcpHeader, mss-(p.tcpHeader-20))
	if limit <= 0 {
		return fmt.Errorf("TCP headers leave no payload within MSS")
	}
	if p.payload() <= limit {
		return emit(p.frame)
	}
	// Authentication and data mappings cannot be copied onto new segments.
	// Unknown options may have the same restriction, so only known options
	// pass above MTU/MSS. See https://www.rfc-editor.org/rfc/rfc2385.html#section-2.
	if p.unsupportedOption {
		return fmt.Errorf("TCP option does not support segmentation")
	}
	if p.tcp[13]&0x26 != 0 || p.tcp[13]&0x10 == 0 {
		return fmt.Errorf("unsupported TCP flags for segmentation")
	}
	headers := 14 + p.ipHeader + p.tcpHeader
	buffer := make([]byte, headers+limit)
	sequence := binary.BigEndian.Uint32(p.tcp[4:])
	for offset := 0; offset < p.payload(); offset += limit {
		end := min(offset+limit, p.payload())
		frame := buffer[:headers+end-offset]
		copy(frame, p.frame[:headers])
		copy(frame[headers:], p.tcp[p.tcpHeader+offset:p.tcpHeader+end])
		ip := frame[14:]
		tcp := ip[p.ipHeader:]
		binary.BigEndian.PutUint32(tcp[4:], sequence+uint32(offset))
		if offset != 0 {
			tcp[13] &^= 0x80
		}
		if end != p.payload() {
			tcp[13] &^= 9
		}
		if p.version == 4 {
			binary.BigEndian.PutUint16(ip[2:], uint16(len(ip)))
			binary.BigEndian.PutUint16(ip[4:], binary.BigEndian.Uint16(p.ip[4:])+uint16(offset/limit))
			ip[10], ip[11] = 0, 0
			binary.BigEndian.PutUint16(ip[10:], tcpChecksum(ip[:20], 0))
		} else {
			binary.BigEndian.PutUint16(ip[4:], uint16(len(tcp)))
		}
		tcp[16], tcp[17] = 0, 0
		sum := uint64(6 + len(tcp))
		if p.version == 4 {
			sum = tcpSum(ip[12:20], sum)
		} else {
			sum = tcpSum(ip[8:40], sum)
		}
		binary.BigEndian.PutUint16(tcp[16:], tcpChecksum(tcp, sum))
		if err := emit(frame); err != nil {
			return err
		}
	}
	return nil
}

func tcpSum(data []byte, sum uint64) uint64 {
	for len(data) >= 2 {
		sum += uint64(binary.BigEndian.Uint16(data))
		data = data[2:]
	}
	if len(data) > 0 {
		sum += uint64(data[0]) << 8
	}
	return sum
}

func tcpChecksum(data []byte, sum uint64) uint16 {
	sum = tcpSum(data, sum)
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}
