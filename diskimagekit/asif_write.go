package diskimagekit

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
)

// asifUpdate owns an unpublished directory and at most one changed table.
// Its new payload and tables never replace storage reachable by the old directory.
type asifUpdate struct {
	layer       *layer
	directory   map[int64]uint64
	inactive    map[int64]uint64
	allocation  *chunkAllocation
	tableIndex  int64
	table       []byte
	bitmapGroup int64
	bitmap      []byte
}

func beginUpdate(l *layer) (*asifUpdate, error) {
	if err := l.loadASIF(); err != nil {
		return nil, err
	}
	if err := l.writable(); err != nil {
		return nil, err
	}
	if l.asif.sequence == math.MaxInt64 {
		return nil, errors.New("diskimagekit: ASIF directory sequence exhausted")
	}
	inactive, _, err := l.readDirectory(1 - l.asif.selected)
	if err != nil {
		return nil, err
	}
	allocation, err := l.allocation(inactive)
	if err != nil {
		return nil, err
	}
	if !l.asif.rotated && l.info.LayerType != Cache {
		id, err := randomUUID()
		if err != nil {
			return nil, err
		}
		if err := writeFullAt(l.io, id[:], 0x20); err != nil {
			return nil, l.failMutation(err)
		}
		if err := l.io.Sync(); err != nil {
			return nil, l.failMutation(err)
		}
		l.info.LayerUUID = formatUUID(id[:])
		l.asif.rotated = true
	}
	return &asifUpdate{
		layer:      l,
		directory:  maps.Clone(l.asif.directory),
		inactive:   inactive,
		allocation: allocation,
		tableIndex: -1,
	}, nil
}

func (l *layer) failMutation(err error) error {
	// A failed write or Sync may have reached storage. Reopen to reload the
	// on-disk directory before choosing chunks to reuse.
	l.mutationErr = fmt.Errorf("diskimagekit: ASIF mutation failed; close and reopen before writing: %w", err)
	return l.mutationErr
}

func (u *asifUpdate) flushTable() error {
	if u.table == nil {
		return nil
	}
	if err := u.flushBitmap(); err != nil {
		return err
	}
	pointer, err := u.allocate(u.table)
	if err != nil {
		return err
	}
	u.directory[u.tableIndex] = pointer
	u.table = nil
	return nil
}

func (u *asifUpdate) selectTable(logical int64) error {
	g := u.layer.asif.geometry
	index := logical / g.covered
	if index != u.tableIndex {
		if err := u.flushTable(); err != nil {
			return err
		}
		table, err := u.layer.tableAt(index)
		if err != nil {
			return err
		}
		u.table = make([]byte, g.chunk)
		copy(u.table, table)
		u.tableIndex = index
	}
	return nil
}

func (u *asifUpdate) setChunk(logical int64, data []byte) error {
	entry := uint64(asifChunkZero) << 62
	if data != nil {
		pointer, err := u.allocate(data)
		if err != nil {
			return err
		}
		entry = uint64(asifChunkData)<<62 | pointer
	}
	return u.setEntry(logical, entry)
}

func (u *asifUpdate) setEntry(logical int64, entry uint64) error {
	if err := u.selectTable(logical); err != nil {
		return err
	}
	g := u.layer.asif.geometry
	relative := logical % g.covered
	offset := (relative/(g.entries*g.chunk))*g.groupBytes + ((relative/g.chunk)%g.entries)*8
	bigEndian.PutUint64(u.table[offset:], entry)
	return nil
}

func (u *asifUpdate) flushBitmap() error {
	if u.bitmap == nil {
		return nil
	}
	pointer, err := u.allocate(u.bitmap)
	if err != nil {
		return err
	}
	g := u.layer.asif.geometry
	bigEndian.PutUint64(u.table[u.bitmapGroup+g.entries*8:], pointer)
	u.bitmap = nil
	return nil
}

func (u *asifUpdate) selectBitmap(group int64) error {
	if u.bitmap != nil && u.bitmapGroup == group {
		return nil
	}
	if err := u.flushBitmap(); err != nil {
		return err
	}
	g := u.layer.asif.geometry
	u.bitmap = make([]byte, g.chunk)
	u.bitmapGroup = group
	// Every sparse entry in this group shares one bitmap. Preserve the other
	// entries' block states when changing just one of them.
	for i := int64(0); i < g.entries; i++ {
		state := asifChunkState(bigEndian.Uint64(u.table[group+i*8:]) >> 62)
		if state != asifChunkBitmap {
			continue
		}
		pointer := bigEndian.Uint64(u.table[group+g.entries*8:])
		offset, err := u.layer.asif.physicalRange(pointer, 0, g.chunk)
		if err != nil {
			return err
		}
		_, err = u.layer.io.ReadAt(u.bitmap, offset)
		return err
	}
	return nil
}

func (u *asifUpdate) setSparseChunk(logical int64, chunk *asifChunk) error {
	if allBlocks(chunk.states, extentData) {
		return u.setChunk(logical, chunk.data)
	}
	if allBlocks(chunk.states, extentZero) {
		return u.setChunk(logical, nil)
	}
	if err := u.selectTable(logical); err != nil {
		return err
	}
	g := u.layer.asif.geometry
	relative := logical % g.covered
	group := relative / (g.entries * g.chunk) * g.groupBytes
	if err := u.selectBitmap(group); err != nil {
		return err
	}
	pointer, err := u.allocate(chunk.data)
	if err != nil {
		return err
	}
	index := (relative / g.chunk) % g.entries
	first := index * (g.chunk / g.block)
	for i, kind := range chunk.states {
		state := asifBlockInherited
		switch kind {
		case extentData:
			state = asifBlockData
		case extentZero:
			state = asifBlockZero
		}
		at := first + int64(i)
		shift := (at % 4) * 2
		u.bitmap[at/4] = u.bitmap[at/4]&^(3<<shift) | byte(state)<<shift
	}
	bigEndian.PutUint64(u.table[group+index*8:], uint64(asifChunkBitmap)<<62|pointer)
	return nil
}

func allBlocks(states []extentKind, kind extentKind) bool {
	for _, state := range states {
		if state != kind {
			return false
		}
	}
	return true
}

type asifChunk struct {
	data   []byte
	states []extentKind
}

func (l *layer) ownChunk(logical int64) (*asifChunk, error) {
	if err := l.loadASIF(); err != nil {
		return nil, err
	}
	g := l.asif.geometry
	chunk := &asifChunk{
		data:   make([]byte, g.chunk),
		states: make([]extentKind, g.chunk/g.block),
	}
	limit := min(g.chunk, l.asif.maximum-logical)
	for cursor := int64(0); cursor < limit; {
		e, err := l.extent(logical + cursor)
		if err != nil {
			return nil, err
		}
		count := min(e.length, limit-cursor)
		if e.kind == extentData {
			if _, err := l.readPayload(chunk.data[cursor:cursor+count], e.offset); err != nil {
				return nil, err
			}
		}
		for sector := cursor / g.block; sector < (cursor+count)/g.block; sector++ {
			chunk.states[sector] = e.kind
		}
		cursor += count
	}
	return chunk, nil
}

func (u *asifUpdate) commit() error {
	if err := u.flushTable(); err != nil {
		return err
	}
	a := u.layer.asif
	next := 1 - a.selected
	if !a.knownDirectory[next] {
		zeros := make([]byte, 32<<10)
		for off := int64(8); off < a.geometry.directoryBytes; {
			count := min(int64(len(zeros)), a.geometry.directoryBytes-off)
			if err := writeFullAt(u.layer.io, zeros[:count], a.directories[next]+off); err != nil {
				return err
			}
			off += count
		}
	}
	// Include entries present only in the inactive directory so removed
	// mappings are cleared before that directory becomes current again.
	changed := maps.Clone(u.inactive)
	for index, pointer := range u.directory {
		changed[index] = pointer
	}
	var value [8]byte
	for _, index := range slices.Sorted(maps.Keys(changed)) {
		if u.directory[index] == u.inactive[index] {
			continue
		}
		bigEndian.PutUint64(value[:], u.directory[index])
		if err := writeFullAt(u.layer.io, value[:], a.directories[next]+8*(index+1)); err != nil {
			return err
		}
	}
	// Publish the sequence only after all referenced data and directory entries
	// are durable. This does not make a torn sequence sector atomic.
	if err := u.layer.io.Sync(); err != nil {
		return err
	}
	sequence := max(a.sequence, 0) + 1
	bigEndian.PutUint64(value[:], uint64(sequence))
	if err := writeFullAt(u.layer.io, value[:], a.directories[next]); err != nil {
		return err
	}
	if err := u.layer.io.Sync(); err != nil {
		return err
	}
	if !a.published[next] {
		bigEndian.PutUint64(value[:], uint64(a.directories[next]))
		if err := writeFullAt(u.layer.io, value[:], 0x10+int64(next)*8); err != nil {
			return err
		}
		if err := u.layer.io.Sync(); err != nil {
			return err
		}
	}
	a.published[next], a.knownDirectory[next] = true, true
	a.directory = u.directory
	a.selected = next
	a.sequence = sequence
	a.physicalSize = u.allocation.highWater
	a.table = nil
	return nil
}

func (d *DiskImage) cacheChunk(index int, logical int64) ([]byte, error) {
	l := d.layers[index]
	if err := l.loadASIF(); err != nil {
		return nil, err
	}
	chunkSize := l.asif.geometry.chunk
	data := make([]byte, chunkSize)
	limit := min(chunkSize, l.asif.maximum-logical)
	for cursor := int64(0); cursor < limit; {
		e, err := l.extent(logical + cursor)
		if err != nil {
			return nil, err
		}
		count := min(e.length, limit-cursor)
		part := data[cursor : cursor+count]
		switch e.kind {
		case extentData:
			if _, err := l.readPayload(part, e.offset); err != nil {
				return nil, err
			}
		case extentInherited:
			if err := d.requireParent(index); err != nil {
				return nil, err
			}
			if _, err := d.read(index-1, part, logical+cursor, false); err != nil {
				return nil, err
			}
		}
		cursor += count
	}
	return data, nil
}

func (d *DiskImage) writeChunk(index int, p []byte, off int64) (*asifChunk, error) {
	l := d.layers[index]
	if err := l.loadASIF(); err != nil {
		return nil, err
	}
	g := l.asif.geometry
	logical := off / g.chunk * g.chunk
	chunk, err := l.ownChunk(logical)
	if err != nil {
		return nil, err
	}
	start := off % g.chunk
	end := min(start+int64(len(p)), g.chunk)
	// Once a block belongs to this layer, reads no longer fall through to its
	// parent. A partial overwrite must copy the parent's untouched bytes first.
	for sector := start / g.block; sector <= (end-1)/g.block; sector++ {
		blockStart := sector * g.block
		blockEnd := blockStart + g.block
		if chunk.states[sector] == extentInherited && (start > blockStart || end < blockEnd) {
			if err := d.requireParent(index); err != nil {
				return nil, err
			}
			if _, err := d.read(index-1, chunk.data[blockStart:blockEnd], logical+blockStart, true); err != nil {
				return nil, err
			}
		}
		chunk.states[sector] = extentData
	}
	copy(chunk.data[start:end], p)
	return chunk, nil
}

func (d *DiskImage) writeASIF(index int, p []byte, off int64) error {
	l := d.layers[index]
	if err := l.loadASIF(); err != nil {
		return err
	}
	chunkSize := l.asif.geometry.chunk
	chunk, err := d.writeChunk(index, p, off)
	if err != nil {
		return err
	}
	u, err := beginUpdate(l)
	if err != nil {
		return err
	}
	for len(p) > 0 {
		logical := off / chunkSize * chunkSize
		count := min(int64(len(p)), chunkSize-off%chunkSize)
		if err := u.setSparseChunk(logical, chunk); err != nil {
			return l.failMutation(err)
		}
		off += count
		p = p[count:]
		if len(p) > 0 {
			chunk, err = d.writeChunk(index, p, off)
			if err != nil {
				return l.failMutation(err)
			}
		}
	}
	if err := u.commit(); err != nil {
		return l.failMutation(err)
	}
	return nil
}

func (d *DiskImage) fillCache(index int, offset int64) error {
	l := d.layers[index]
	chunkSize := l.asif.geometry.chunk
	u, err := beginUpdate(l)
	if err != nil {
		return err
	}
	logical := offset / chunkSize * chunkSize
	data, err := d.cacheChunk(index, logical)
	if err != nil {
		return l.failMutation(err)
	}
	if err := u.setChunk(logical, data); err != nil {
		return l.failMutation(err)
	}
	if err := u.commit(); err != nil {
		return l.failMutation(err)
	}
	return nil
}

func (d *DiskImage) truncateASIF(size int64) error {
	index := len(d.layers) - 1
	l := d.layers[index]
	if err := l.loadASIF(); err != nil {
		return err
	}
	maximum := l.asif.maximum
	if l.asif.metadata != 0 {
		maximum = l.asif.metadata
	}
	if size > maximum {
		return errors.New("diskimagekit: image exceeds ASIF data capacity")
	}
	if size < l.size {
		u, err := beginUpdate(l)
		if err != nil {
			return err
		}
		if err := u.discardAfter(size); err != nil {
			return l.failMutation(err)
		}
		if err := u.commit(); err != nil {
			return l.failMutation(err)
		}
	}
	var count [8]byte
	bigEndian.PutUint64(count[:], uint64(size/int64(l.info.BlockSize)))
	if err := writeFullAt(l.io, count[:], 0x30); err != nil {
		return l.failMutation(err)
	}
	if err := l.io.Sync(); err != nil {
		return l.failMutation(err)
	}
	l.size = size
	l.info.BlockCount = size / int64(l.info.BlockSize)
	return nil
}

func (u *asifUpdate) discardAfter(size int64) error {
	l := u.layer
	g := l.asif.geometry
	firstTable, lastTable := size/g.covered, (l.size-1)/g.covered
	for _, index := range slices.Sorted(maps.Keys(l.asif.directory)) {
		if index < firstTable || index > lastTable {
			continue
		}
		table, err := l.tableAt(index)
		if err != nil {
			return err
		}
		start := index * g.covered
		end := start + min(g.covered, l.size-start)
		for logical := max(start, size/g.chunk*g.chunk); logical < end; {
			relative := logical % g.covered
			group := relative / (g.entries * g.chunk) * g.groupBytes
			entry := (relative / g.chunk) % g.entries
			state := asifChunkState(bigEndian.Uint64(table[group+entry*8:]) >> 62)
			first := max(size-logical, 0)
			last := min(l.size-logical, g.chunk)
			if state != asifChunkZero && first == 0 && last == g.chunk {
				if err := u.setChunk(logical, nil); err != nil {
					return err
				}
			} else if state == asifChunkData || state == asifChunkBitmap {
				chunk, err := l.ownChunk(logical)
				if err != nil {
					return err
				}
				clear(chunk.data[first:last])
				for sector := first / g.block; sector < last/g.block; sector++ {
					chunk.states[sector] = extentZero
				}
				if !slices.Contains(chunk.states, extentData) {
					entry := uint64(0)
					if chunk.states[0] == extentZero {
						entry = uint64(asifChunkZero) << 62
					}
					if err := u.setEntry(logical, entry); err != nil {
						return err
					}
				} else if err := u.setSparseChunk(logical, chunk); err != nil {
					return err
				}
			}
			if end-logical <= g.chunk {
				break
			}
			logical += g.chunk
		}
	}
	return nil
}
