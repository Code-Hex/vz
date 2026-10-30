package vmnetsock

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"sync"
	"time"
)

const (
	ethernetHeaderSize = 14
	etherTypeOffset    = 12
	etherTypeIPv4      = 0x0800
	etherTypeIPv6      = 0x86dd
	etherTypeVLAN      = 0x8100
	etherTypeQinQ      = 0x88a8

	ipv4Version              = 4
	ipv4HeaderSize           = 20
	ipv4TotalLengthOffset    = 2
	ipv4IdentificationOffset = 4
	ipv4FragmentFlagsOffset  = 6
	ipv4ProtocolOffset       = 9
	ipv4ChecksumOffset       = 10
	ipv4SourceOffset         = 12
	ipv4DestinationOffset    = 16
	ipv4MoreFragments        = 0x2000
	ipv4FragmentOffsetMask   = 0x1fff
	defaultIPv4MSS           = 536

	ipv6Version             = 6
	ipv6HeaderSize          = 40
	ipv6PayloadLengthOffset = 4
	ipv6NextHeaderOffset    = 6
	ipv6SourceOffset        = 8
	ipv6DestinationOffset   = 24
	defaultIPv6MSS          = 1220

	ipProtocolTCP          = 6
	ipv6HopByHop           = 0
	ipv6Routing            = 43
	ipv6Fragment           = 44
	ipv6Authentication     = 51
	ipv6DestinationOptions = 60

	tcpHeaderSize            = 20
	tcpDestinationPortOffset = 2
	tcpSequenceOffset        = 4
	tcpAcknowledgmentOffset  = 8
	tcpHeaderLengthOffset    = 12
	tcpFlagsOffset           = 13
	tcpChecksumOffset        = 16

	tcpFlagFIN = 0x01
	tcpFlagSYN = 0x02
	tcpFlagRST = 0x04
	tcpFlagPSH = 0x08
	tcpFlagACK = 0x10
	tcpFlagURG = 0x20
	tcpFlagCWR = 0x80

	tcpOptionEnd           = 0
	tcpOptionNOP           = 1
	tcpOptionMSS           = 2
	tcpOptionWindowScale   = 3
	tcpOptionSACKPermitted = 4
	tcpOptionSACK          = 5
	tcpOptionTimestamp     = 8
	tcpOptionMSSLength     = 4

	peerDirection   = 0
	nativeDirection = 1
	bothFINs        = 1<<peerDirection | 1<<nativeDirection
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
	flows map[tcpFlowKey]*tcpFlow
}

func newTCPSegmentation(mtu int) *tcpSegmentation {
	return &tcpSegmentation{
		mtu:   mtu,
		flows: make(map[tcpFlowKey]*tcpFlow),
	}
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
		if len(frame) > s.mtu+ethernetHeaderSize {
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
	direction := peerDirection
	if !peer {
		key.peer, key.remote = key.remote, key.peer
		direction = nativeDirection
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, known := s.flows[key]
	// Validate updates on a copy before committing them to a live flow.
	var f tcpFlow
	if known {
		f = *entry
	}
	if known && f.expired(now) {
		delete(s.flows, key)
		entry = nil
		known = false
	}
	flags := p.tcp[tcpFlagsOffset]
	if flags&tcpFlagRST != 0 {
		if p.payload() != 0 {
			return 0, fmt.Errorf("TCP reset contains data")
		}
		// The opposite pump can still receive data sent before the reset.
		if known && f.closedAt.IsZero() {
			f.closedAt = now
			*entry = f
		}
		return 0, nil
	}
	seq := binary.BigEndian.Uint32(p.tcp[tcpSequenceOffset:])
	ack := binary.BigEndian.Uint32(p.tcp[tcpAcknowledgmentOffset:])
	if flags&tcpFlagSYN != 0 {
		if !known && flags&tcpFlagACK == 0 {
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
	// TCP sequence ordering uses the signed distance between two 32-bit values.
	if flags&tcpFlagACK != 0 && f.finSeen&(1<<opposite) != 0 && int32(ack-f.fin[opposite]) >= 0 {
		f.finAcked |= 1 << opposite
	}
	if flags&tcpFlagFIN != 0 {
		// FIN occupies one sequence number after the payload.
		end := seq + uint32(p.payload()) + 1
		if f.finSeen&(1<<direction) != 0 && f.fin[direction] != end {
			return 0, fmt.Errorf("TCP FIN retransmission changed sequence")
		}
		f.fin[direction] = end
		f.finSeen |= 1 << direction
	}
	// Keep the MSS for FIN/data retransmission after both half-closes finish.
	if f.finAcked == bothFINs && f.closedAt.IsZero() {
		f.closedAt = now
	}
	if entry == nil {
		entry = new(tcpFlow)
		s.flows[key] = entry
	}
	*entry = f
	return f.mss, nil
}

func (f tcpFlow) observeSYN(p *tcpPacket, peer, known bool) (tcpFlow, bool, error) {
	if p.payload() != 0 {
		return f, known, fmt.Errorf("TCP SYN data is not supported for segmentation")
	}
	seq := binary.BigEndian.Uint32(p.tcp[tcpSequenceOffset:])
	ack := binary.BigEndian.Uint32(p.tcp[tcpAcknowledgmentOffset:])
	if p.tcp[tcpFlagsOffset]&tcpFlagACK == 0 {
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
	if len(frame) < ethernetHeaderSize {
		return nil, fmt.Errorf("short Ethernet frame")
	}
	p := &tcpPacket{frame: frame, ip: frame[ethernetHeaderSize:]}
	var src, dst netip.Addr
	switch binary.BigEndian.Uint16(frame[etherTypeOffset:]) {
	case etherTypeVLAN, etherTypeQinQ:
		return nil, fmt.Errorf("VLAN frames are not supported with TCP segmentation")
	case etherTypeIPv4:
		if len(p.ip) < ipv4HeaderSize || p.ip[0]>>4 != ipv4Version {
			return nil, fmt.Errorf("invalid IPv4 header")
		}
		p.version = ipv4Version
		p.ipHeader = int(p.ip[0]&0x0f) * 4
		n := int(binary.BigEndian.Uint16(p.ip[ipv4TotalLengthOffset:]))
		if p.ipHeader < ipv4HeaderSize || n < p.ipHeader || n > len(p.ip) {
			return nil, fmt.Errorf("invalid IPv4 length")
		}
		p.ip = p.ip[:n]
		if p.ip[ipv4ProtocolOffset] != ipProtocolTCP {
			return nil, nil
		}
		fragmentFlags := binary.BigEndian.Uint16(p.ip[ipv4FragmentFlagsOffset:])
		if p.ipHeader != ipv4HeaderSize || fragmentFlags&(ipv4MoreFragments|ipv4FragmentOffsetMask) != 0 {
			return nil, fmt.Errorf("TCP IPv4 options and fragments are not supported")
		}
		src = netip.AddrFrom4([4]byte(p.ip[ipv4SourceOffset:ipv4DestinationOffset]))
		dst = netip.AddrFrom4([4]byte(p.ip[ipv4DestinationOffset:ipv4HeaderSize]))
	case etherTypeIPv6:
		if len(p.ip) < ipv6HeaderSize || p.ip[0]>>4 != ipv6Version {
			return nil, fmt.Errorf("invalid IPv6 header")
		}
		p.version = ipv6Version
		p.ipHeader = ipv6HeaderSize
		n := ipv6HeaderSize + int(binary.BigEndian.Uint16(p.ip[ipv6PayloadLengthOffset:]))
		if n > len(p.ip) {
			return nil, fmt.Errorf("invalid IPv6 length")
		}
		p.ip = p.ip[:n]
		isTCP, err := ipv6TCPHeader(p.ip)
		if err != nil || !isTCP {
			return nil, err
		}
		src = netip.AddrFrom16([16]byte(p.ip[ipv6SourceOffset:ipv6DestinationOffset]))
		dst = netip.AddrFrom16([16]byte(p.ip[ipv6DestinationOffset:ipv6HeaderSize]))
	default:
		return nil, nil
	}
	p.frame = frame[:ethernetHeaderSize+len(p.ip)]
	p.tcp = p.ip[p.ipHeader:]
	if len(p.tcp) < tcpHeaderSize {
		return nil, fmt.Errorf("short TCP header")
	}
	p.tcpHeader = int(p.tcp[tcpHeaderLengthOffset]>>4) * 4
	if p.tcpHeader < tcpHeaderSize || p.tcpHeader > len(p.tcp) {
		return nil, fmt.Errorf("invalid TCP header length")
	}
	p.src = netip.AddrPortFrom(src, binary.BigEndian.Uint16(p.tcp))
	p.dst = netip.AddrPortFrom(dst, binary.BigEndian.Uint16(p.tcp[tcpDestinationPortOffset:]))
	if err := p.parseOptions(); err != nil {
		return nil, err
	}
	return p, nil
}

func ipv6TCPHeader(ip []byte) (bool, error) {
	next := ip[ipv6NextHeaderOffset]
	offset := ipv6HeaderSize
	for next != ipProtocolTCP {
		switch next {
		case ipv6HopByHop, ipv6Routing, ipv6DestinationOptions, ipv6Authentication:
		case ipv6Fragment:
			return false, fmt.Errorf("IPv6 fragments are not supported with TCP segmentation")
		default:
			return false, nil
		}
		if offset+2 > len(ip) {
			return false, fmt.Errorf("short IPv6 extension")
		}
		// Extension lengths use eight-byte units. AH uses four-byte units.
		length := (int(ip[offset+1]) + 1) * 8
		if next == ipv6Authentication {
			length = (int(ip[offset+1]) + 2) * 4
		}
		if offset+length > len(ip) {
			return false, fmt.Errorf("invalid IPv6 extension length")
		}
		next = ip[offset]
		offset += length
	}
	if offset != ipv6HeaderSize {
		return false, fmt.Errorf("TCP IPv6 extensions are not supported")
	}
	return true, nil
}

func (p *tcpPacket) parseOptions() error {
	// RFC 9293 section 3.7.1 supplies these defaults only when an observed
	// SYN omits MSS. A connection whose SYN was missed never reaches tcpReady.
	// https://www.rfc-editor.org/rfc/rfc9293.html#section-3.7.1
	p.mss = defaultIPv4MSS
	if p.version == ipv6Version {
		p.mss = defaultIPv6MSS
	}
	seenMSS := false
	for options := p.tcp[tcpHeaderSize:p.tcpHeader]; len(options) > 0; {
		kind := options[0]
		if kind == tcpOptionEnd {
			break
		}
		if kind == tcpOptionNOP {
			options = options[1:]
			continue
		}
		if len(options) < 2 || options[1] < 2 || int(options[1]) > len(options) {
			return fmt.Errorf("invalid TCP option length")
		}
		switch kind {
		case tcpOptionMSS, tcpOptionWindowScale, tcpOptionSACKPermitted, tcpOptionSACK, tcpOptionTimestamp:
			// MSS, window scale, SACK, and timestamps survive segmentation.
		default:
			p.unsupportedOption = true
		}
		if kind == tcpOptionMSS {
			if options[1] != tcpOptionMSSLength || seenMSS {
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
	limit := min(mtu-p.ipHeader-p.tcpHeader, mss-(p.tcpHeader-tcpHeaderSize))
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
	flags := p.tcp[tcpFlagsOffset]
	if flags&(tcpFlagSYN|tcpFlagRST|tcpFlagURG) != 0 || flags&tcpFlagACK == 0 {
		return fmt.Errorf("unsupported TCP flags for segmentation")
	}
	headers := ethernetHeaderSize + p.ipHeader + p.tcpHeader
	buffer := make([]byte, headers+limit)
	sequence := binary.BigEndian.Uint32(p.tcp[tcpSequenceOffset:])
	for offset := 0; offset < p.payload(); offset += limit {
		end := min(offset+limit, p.payload())
		frame := buffer[:headers+end-offset]
		copy(frame, p.frame[:headers])
		copy(frame[headers:], p.tcp[p.tcpHeader+offset:p.tcpHeader+end])
		ip := frame[ethernetHeaderSize:]
		tcp := ip[p.ipHeader:]
		binary.BigEndian.PutUint32(tcp[tcpSequenceOffset:], sequence+uint32(offset))
		// Carry CWR on the first segment. FIN and PSH follow the final payload byte.
		if offset != 0 {
			tcp[tcpFlagsOffset] &^= tcpFlagCWR
		}
		if end != p.payload() {
			tcp[tcpFlagsOffset] &^= tcpFlagFIN | tcpFlagPSH
		}
		p.updateSegmentHeaders(ip, offset/limit)
		if err := emit(frame); err != nil {
			return err
		}
	}
	return nil
}

func (p *tcpPacket) updateSegmentHeaders(ip []byte, segmentIndex int) {
	tcp := ip[p.ipHeader:]
	if p.version == ipv4Version {
		binary.BigEndian.PutUint16(ip[ipv4TotalLengthOffset:], uint16(len(ip)))
		id := binary.BigEndian.Uint16(p.ip[ipv4IdentificationOffset:]) + uint16(segmentIndex)
		binary.BigEndian.PutUint16(ip[ipv4IdentificationOffset:], id)
		binary.BigEndian.PutUint16(ip[ipv4ChecksumOffset:], 0)
		binary.BigEndian.PutUint16(ip[ipv4ChecksumOffset:], tcpChecksum(ip[:ipv4HeaderSize], 0))
	} else {
		binary.BigEndian.PutUint16(ip[ipv6PayloadLengthOffset:], uint16(len(tcp)))
	}
	// TCP's pseudo-header includes the segment length, which changes when split.
	// https://www.rfc-editor.org/rfc/rfc9293.html#section-3.1
	binary.BigEndian.PutUint16(tcp[tcpChecksumOffset:], 0)
	sum := uint64(ipProtocolTCP + len(tcp))
	if p.version == ipv4Version {
		sum = tcpSum(ip[ipv4SourceOffset:ipv4HeaderSize], sum)
	} else {
		sum = tcpSum(ip[ipv6SourceOffset:ipv6HeaderSize], sum)
	}
	binary.BigEndian.PutUint16(tcp[tcpChecksumOffset:], tcpChecksum(tcp, sum))
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
