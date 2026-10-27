package diskimagekit

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
)

const (
	chunkSize    int64  = 1 << 20
	maximumSize  int64  = 1 << 52
	physicalMask uint64 = 1<<55 - 1
)

var bigEndian = binary.BigEndian

type geometry struct {
	block          int64
	chunk          int64
	entries        int64
	groupBytes     int64
	covered        int64
	tableCount     int64
	directoryBytes int64
}

func newGeometry(block BlockSize, maximum uint64, chunk int64) geometry {
	// A bitmap uses two bits per block. One bitmap chunk therefore covers
	// 4*block data chunks; its pointer follows those entries in the table group.
	entries := 4 * int64(block)
	groupBytes := 8 * (entries + 1)
	covered := (chunk / groupBytes) * entries * chunk
	var tables int64
	if maximum != 0 {
		tables = int64((maximum-1)/uint64(covered) + 1)
	}
	directoryBytes := (8*(tables+1) + int64(block) - 1) / int64(block) * int64(block)
	return geometry{
		block:          int64(block),
		chunk:          chunk,
		entries:        entries,
		groupBytes:     groupBytes,
		covered:        covered,
		tableCount:     tables,
		directoryBytes: directoryBytes,
	}
}

type asifImage struct {
	header         []byte
	geometry       geometry
	directories    [2]int64
	published      [2]bool
	knownDirectory [2]bool
	selected       int
	sequence       int64
	directory      map[int64]uint64
	metadata       int64
	maximum        int64
	physicalSize   int64
	reserved       [3]chunkRange
	readOnly       bool
	rotated        bool
	tableOffset    int64
	table          []byte
}

type chunkRange struct {
	first uint64
	end   uint64
}

func chunkRangeFor(offset, length, chunk int64) chunkRange {
	return chunkRange{
		first: uint64(offset / chunk),
		end:   uint64((offset+length-1)/chunk + 1),
	}
}

func (a *asifImage) reservedEnd(pointer uint64) uint64 {
	for _, region := range a.reserved {
		if pointer >= region.first && pointer < region.end {
			return region.end
		}
	}
	return pointer
}

// asifChunkState is the two-bit tag in an ASIF table entry.
type asifChunkState uint64

const (
	asifChunkInherited asifChunkState = 0
	asifChunkData      asifChunkState = 1
	asifChunkZero      asifChunkState = 2
	asifChunkBitmap    asifChunkState = 3
)

// asifBlockState is the two-bit value for one block in an ASIF bitmap.
type asifBlockState uint8

const (
	asifBlockInherited asifBlockState = 0
	asifBlockData      asifBlockState = 1
	asifBlockZero      asifBlockState = 2
	asifBlockInvalid   asifBlockState = 3
)

type extentKind uint8

const (
	extentInherited extentKind = iota
	extentZero
	extentData
)

type extent struct {
	kind   extentKind
	offset int64
	length int64
}

func invalid(reason string) error {
	return fmt.Errorf("%w: %s", ErrInvalidImage, reason)
}

func isASIF(header []byte) bool {
	if len(header) < 0x68 || string(header[:4]) != "shdw" {
		return false
	}
	return bigEndian.Uint32(header[12:16]) <= 1 &&
		bigEndian.Uint16(header[0x46:0x48]) == 0 &&
		bigEndian.Uint32(header[0x64:0x68]) == 0
}

func (l *layer) parseASIF() error {
	header := make([]byte, 512)
	if _, err := l.io.ReadAt(header, 0); err != nil {
		return fmt.Errorf("%w: header: %v", ErrInvalidImage, err)
	}
	if bigEndian.Uint32(header[4:8]) != 1 {
		return invalid("unsupported ASIF version")
	}
	block := BlockSize(bigEndian.Uint16(header[0x44:0x46]))
	if block != BlockSize512 && block != BlockSize4096 {
		return invalid("unsupported ASIF block size")
	}
	headerSize := int64(bigEndian.Uint32(header[8:12]))
	if headerSize < 80 || headerSize > 1<<20 || headerSize%int64(block) != 0 {
		return invalid("invalid ASIF header size")
	}
	if headerSize > int64(len(header)) {
		header = make([]byte, headerSize)
		if _, err := l.io.ReadAt(header, 0); err != nil {
			return invalid("truncated ASIF header")
		}
	}
	for _, value := range header[0x6c:] {
		if value != 0 {
			return invalid("nonzero reserved header field")
		}
	}
	chunk := int64(bigEndian.Uint32(header[0x40:0x44]))
	if chunk%int64(block) != 0 || chunk < int64(block) || chunk > 64<<20 {
		return invalid("unsupported ASIF chunk size")
	}
	blocks := bigEndian.Uint64(header[0x30:0x38])
	maximumBlocks := bigEndian.Uint64(header[0x38:0x40])
	if maximumBlocks >= uint64(math.MaxUint64)/uint64(block) || blocks > maximumBlocks {
		return invalid("invalid logical size")
	}
	if blocks > uint64(math.MaxInt64)/uint64(block) {
		return invalid("logical size exceeds Go file offsets")
	}
	l.asif = &asifImage{
		header:       header,
		physicalSize: l.size,
		readOnly:     bigEndian.Uint32(header[0x60:0x64]) != 0,
	}
	l.info.Format = ASIF
	l.info.BlockCount = int64(blocks)
	l.info.BlockSize = block
	l.info.LayerUUID = formatUUID(header[0x20:0x30])
	l.info.ParentUUID = formatUUID(header[0x50:0x60])
	if bigEndian.Uint32(header[12:16]) == 1 {
		l.info.LayerType = Cache
	}
	l.size = int64(blocks) * int64(block)
	return nil
}

func (l *layer) loadASIF() error {
	if l.asif == nil || l.asif.header == nil {
		return nil
	}
	previous := l.asif
	if err := l.initASIF(previous.header); err != nil {
		l.asif = previous
		return err
	}
	return nil
}

func (l *layer) initASIF(header []byte) error {
	block := l.info.BlockSize
	chunk := int64(bigEndian.Uint32(header[0x40:0x44]))
	if chunk < 8*(4*int64(block)+1) {
		return invalid("ASIF chunk cannot hold a table group")
	}
	maximum := bigEndian.Uint64(header[0x38:0x40]) * uint64(block)
	maxOffset := uint64(math.MaxInt64) / uint64(block) * uint64(block)
	headerSize := int64(len(header))
	physicalSize := l.asif.physicalSize
	metadataChunk := bigEndian.Uint64(header[0x48:0x50])
	if metadataChunk != 0 && metadataChunk >= maximum/uint64(chunk) {
		return invalid("invalid metadata location")
	}
	metadata := metadataChunk * uint64(chunk)
	if metadata != 0 && uint64(l.size) > metadata {
		return invalid("logical data overlaps metadata")
	}
	g := newGeometry(block, maximum, chunk)
	var dirs [2]int64
	var published [2]bool
	headerEnd := max(headerSize, 512)
	for i := range dirs {
		value := bigEndian.Uint64(header[0x10+i*8 : 0x18+i*8])
		published[i] = value != 0
		if value == 0 {
			value = uint64((int64(512)+int64(block)-1)/int64(block)*int64(block) + int64(i)*g.directoryBytes)
		}
		if value > uint64(math.MaxInt64-g.directoryBytes) || value < uint64(headerEnd) || value%uint64(block) != 0 {
			return invalid("invalid directory offset")
		}
		dirs[i] = int64(value)
	}
	if dirs[0] < dirs[1]+g.directoryBytes && dirs[1] < dirs[0]+g.directoryBytes {
		return invalid("overlapping directories")
	}
	directoryEnd := max(dirs[0]+g.directoryBytes, dirs[1]+g.directoryBytes)
	if directoryEnd > physicalSize {
		return invalid("directory exceeds file")
	}
	a := &asifImage{
		geometry:       g,
		directories:    dirs,
		published:      published,
		knownDirectory: published,
		metadata:       int64(min(metadata, maxOffset)),
		maximum:        int64(min(maximum, maxOffset)),
		physicalSize:   physicalSize,
		reserved: [3]chunkRange{
			chunkRangeFor(0, headerEnd, chunk),
			chunkRangeFor(dirs[0], g.directoryBytes, chunk),
			chunkRangeFor(dirs[1], g.directoryBytes, chunk),
		},
		readOnly: bigEndian.Uint32(header[0x60:0x64]) != 0,
	}
	if !published[0] && !published[1] {
		a.knownDirectory[0] = true
	}
	l.asif = a
	first, sequence0, err := l.readDirectory(0)
	if err != nil {
		return err
	}
	second, sequence1, err := l.readDirectory(1)
	if err != nil {
		return err
	}
	a.directory, a.sequence = first, sequence0
	if a.knownDirectory[1] && (!a.knownDirectory[0] || sequence1 > sequence0) {
		a.selected, a.directory, a.sequence = 1, second, sequence1
	}
	for _, pointer := range a.directory {
		if _, err := a.physicalOffset(pointer); err != nil {
			return err
		}
	}
	metadata, err = l.metadataLimit(metadata, maximum)
	if err != nil {
		return err
	}
	a.metadata = int64(min(metadata, maxOffset))
	return nil
}

func (l *layer) metadataLimit(offset, maximum uint64) (uint64, error) {
	// Metadata links need not increase in offset. Growing the image must stop
	// before the lowest metadata chunk, even when it occurs later in the chain.
	limit := offset
	seen := make(map[uint64]struct{})
	for offset != 0 {
		if _, ok := seen[offset]; ok {
			break
		}
		seen[offset] = struct{}{}
		limit = min(limit, offset)
		if offset < uint64(l.size) {
			return 0, invalid("logical data overlaps metadata")
		}
		e, err := l.asifExtent(offset)
		if err != nil {
			return 0, err
		}
		if e.kind != extentData {
			break
		}
		var header [28]byte
		if _, err := l.readPayload(header[:], e.offset); err != nil {
			return 0, err
		}
		if string(header[:4]) != "meta" || bigEndian.Uint32(header[4:8]) != 1 {
			break
		}
		next := bigEndian.Uint64(header[20:28])
		chunk := uint64(l.asif.geometry.chunk)
		if next != 0 && next >= maximum/chunk {
			return 0, invalid("invalid metadata chain location")
		}
		offset = next * chunk
	}
	return limit, nil
}

func (l *layer) readDirectory(which int) (map[int64]uint64, int64, error) {
	a := l.asif
	if !a.knownDirectory[which] {
		return make(map[int64]uint64), 0, nil
	}
	entries := a.geometry.tableCount + 1
	result := make(map[int64]uint64)
	var sequence int64
	buffer := make([]byte, 32<<10)
	for start := int64(0); start < entries; {
		count := min(entries-start, int64(len(buffer)/8))
		data := buffer[:count*8]
		if _, err := l.io.ReadAt(data, a.directories[which]+start*8); err != nil {
			return nil, 0, fmt.Errorf("%w: directory: %v", ErrInvalidImage, err)
		}
		for i := int64(0); i < count; i++ {
			value := bigEndian.Uint64(data[i*8:])
			if start+i == 0 {
				sequence = int64(value)
			} else if value != 0 {
				result[start+i-1] = value
			}
		}
		start += count
	}
	return result, sequence, nil
}

func (a *asifImage) physicalOffset(pointer uint64) (int64, error) {
	if pointer == 0 || pointer > uint64(math.MaxInt64/a.geometry.chunk) {
		return 0, invalid("invalid physical chunk")
	}
	return int64(pointer) * a.geometry.chunk, nil
}

func (a *asifImage) physicalRange(pointer uint64, within, count int64) (int64, error) {
	offset, err := a.physicalOffset(pointer)
	if err != nil {
		return 0, err
	}
	if within < 0 || count < 0 || within > a.physicalSize || offset > a.physicalSize-within || count > a.physicalSize-offset-within {
		return 0, invalid("physical range exceeds file")
	}
	return offset + within, nil
}

func (l *layer) readTable(pointer uint64, table []byte) (int64, error) {
	offset, err := l.asif.physicalOffset(pointer)
	if err != nil {
		return 0, err
	}
	clear(table)
	if _, err := l.io.ReadAt(table, offset); err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	return offset, nil
}

func (l *layer) readPayload(data []byte, offset int64) (int, error) {
	n, err := l.io.ReadAt(data, offset)
	if errors.Is(err, io.EOF) {
		clear(data[n:])
		return len(data), nil
	}
	return n, err
}

func (l *layer) tableAt(index int64) ([]byte, error) {
	a := l.asif
	pointer := a.directory[index]
	if pointer == 0 {
		return nil, nil
	}
	offset, err := a.physicalOffset(pointer)
	if err != nil {
		return nil, err
	}
	if a.table != nil && a.tableOffset == offset {
		return a.table, nil
	}
	table := make([]byte, a.geometry.chunk)
	if _, err := l.readTable(pointer, table); err != nil {
		return nil, err
	}
	a.tableOffset = offset
	a.table = table
	return table, nil
}

func (l *layer) extent(offset int64) (extent, error) {
	if err := l.loadASIF(); err != nil {
		return extent{}, err
	}
	if l.asif == nil {
		return extent{
			kind:   extentData,
			offset: offset,
			length: l.size - offset,
		}, nil
	}
	return l.asifExtent(uint64(offset))
}

func (l *layer) asifExtent(offset uint64) (extent, error) {
	a := l.asif
	g := a.geometry
	within := int64(offset % uint64(g.chunk))
	length := g.chunk - within
	table, err := l.tableAt(int64(offset / uint64(g.covered)))
	if err != nil {
		return extent{}, err
	}
	if table == nil {
		return extent{
			kind:   extentInherited,
			length: length,
		}, nil
	}
	relative := int64(offset % uint64(g.covered))
	group := (relative / (g.entries * g.chunk)) * g.groupBytes
	index := (relative / g.chunk) % g.entries
	entry := bigEndian.Uint64(table[group+index*8:])
	if entry & ^(physicalMask|3<<62) != 0 {
		return extent{}, invalid("unknown chunk flags")
	}
	physical := entry & physicalMask
	chunkState := asifChunkState(entry >> 62)
	switch chunkState {
	case asifChunkInherited, asifChunkZero:
		kind := extentInherited
		if chunkState == asifChunkZero {
			kind = extentZero
		}
		return extent{
			kind:   kind,
			length: length,
		}, nil
	case asifChunkBitmap:
		bitmap := bigEndian.Uint64(table[group+g.entries*8:])
		sector := index*(g.chunk/g.block) + within/g.block
		bitmapOffset, err := a.physicalRange(bitmap, 0, g.chunk)
		if err != nil {
			return extent{}, err
		}
		var b [1]byte
		if _, err := l.io.ReadAt(b[:], bitmapOffset+sector/4); err != nil {
			return extent{}, err
		}
		blockState := asifBlockState((b[0] >> ((sector % 4) * 2)) & 3)
		length = g.block - within%g.block
		switch blockState {
		case asifBlockInherited:
			return extent{
				kind:   extentInherited,
				length: length,
			}, nil
		case asifBlockZero:
			return extent{
				kind:   extentZero,
				length: length,
			}, nil
		case asifBlockInvalid:
			return extent{}, invalid("invalid bitmap state")
		}
	}
	if physical == 0 {
		return extent{
			kind:   extentZero,
			length: length,
		}, nil
	}
	physicalOffset, err := a.physicalOffset(physical)
	if err != nil {
		return extent{}, err
	}
	if physicalOffset > math.MaxInt64-within {
		return extent{}, invalid("physical offset overflow")
	}
	physicalOffset += within
	return extent{
		kind:   extentData,
		offset: physicalOffset,
		length: length,
	}, nil
}

func randomUUID() ([16]byte, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return id, err
	}
	id[6] = id[6]&15 | 64
	id[8] = id[8]&63 | 128
	return id, nil
}

func formatUUID(id []byte) string {
	var nonzero byte
	for _, b := range id {
		nonzero |= b
	}
	if nonzero == 0 {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
}

func createASIF(path string, blocks int64, block BlockSize, kind LayerType, parent string) (*layer, error) {
	size, err := byteSize(blocks, block)
	if err != nil {
		return nil, err
	}
	if size > maximumSize-chunkSize {
		return nil, errors.New("diskimagekit: image exceeds ASIF data capacity")
	}
	id, err := randomUUID()
	if err != nil {
		return nil, err
	}
	stable, err := randomUUID()
	if err != nil {
		return nil, err
	}
	var parentID []byte
	if parent != "" {
		parentID, err = hex.DecodeString(strings.ReplaceAll(parent, "-", ""))
		if err != nil || len(parentID) != 16 {
			return nil, errors.New("diskimagekit: invalid parent UUID")
		}
		if kind == Cache {
			copy(id[:], parentID)
		}
	}
	g := newGeometry(block, uint64(maximumSize), chunkSize)
	// Keep the initial image compact. The header and directories occupy chunk 0,
	// followed by a table, metadata payload, and bitmap. The table maps metadata
	// near the end of the logical address space to these physical chunks.
	data := make([]byte, 4*chunkSize)
	put64 := func(offset int64, value uint64) { bigEndian.PutUint64(data[offset:offset+8], value) }
	put32 := func(offset int64, value uint32) { bigEndian.PutUint32(data[offset:offset+4], value) }
	copy(data, "shdw")
	put32(4, 1)
	put32(8, uint32(block))
	if kind == Cache {
		put32(12, 1)
	}
	dirs := [2]int64{int64(block), int64(block) + g.directoryBytes}
	put64(0x10, uint64(dirs[0]))
	put64(0x18, uint64(dirs[1]))
	copy(data[0x20:0x30], id[:])
	put64(0x30, uint64(blocks))
	put64(0x38, uint64(maximumSize/int64(block)))
	put32(0x40, uint32(chunkSize))
	bigEndian.PutUint16(data[0x44:0x46], uint16(block))
	metadata := maximumSize - chunkSize
	put64(0x48, uint64(metadata/chunkSize))
	copy(data[0x50:0x60], parentID)
	put64(dirs[1], 1)
	for _, dir := range dirs {
		put64(dir+8*g.tableCount, 1)
	}
	relative := metadata % g.covered
	index := (relative / chunkSize) % g.entries
	group := chunkSize + (relative/(g.entries*chunkSize))*g.groupBytes
	put64(group+index*8, uint64(asifChunkBitmap)<<62|2)
	put64(group+g.entries*8, 3)
	data[3*chunkSize+index*(chunkSize/int64(block))/4] = byte(asifBlockData | asifBlockData<<2)
	copy(data[2*chunkSize:], "meta")
	put32(2*chunkSize+4, 1)
	put32(2*chunkSize+8, uint32(block))
	put64(2*chunkSize+12, uint64(block))
	plist := fmt.Sprintf("<?xml version=\"1.0\" encoding=\"UTF-8\"?><!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\"><plist version=\"1.0\"><dict><key>internal metadata</key><dict><key>stable uuid</key><string>%s</string></dict><key>user metadata</key><dict/></dict></plist>", formatUUID(stable[:]))
	copy(data[2*chunkSize+int64(block):], plist)
	var l *layer
	_, err = createFile(path, func(f *os.File) error {
		if err := writeFullAt(f, data, 0); err != nil {
			return err
		}
		l = &layer{
			file: f,
			io:   f,
			size: int64(len(data)),
			info: LayerInfo{
				Path:      path,
				OpenMode:  ReadWrite,
				LayerType: kind,
			},
		}
		return l.parseASIF()
	})
	if err != nil {
		return nil, err
	}
	return l, nil
}

func writeFullAt(storage imageIO, data []byte, offset int64) error {
	n, err := storage.WriteAt(data, offset)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}
