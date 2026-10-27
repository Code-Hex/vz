package diskimagekit

import (
	"errors"
	"math"
)

// chunkAllocation protects both directory generations until a new one is
// published. It is rebuilt for each update, including after reopening a file.
type chunkAllocation struct {
	used      map[uint64]struct{}
	cursor    uint64
	highWater int64
}

func (l *layer) allocation(inactive map[int64]uint64) (*chunkAllocation, error) {
	a := l.asif
	used := make(map[uint64]struct{})
	if err := l.markDirectory(a.directory, used); err != nil {
		return nil, err
	}
	cursor := uint64(0)
	if err := l.markDirectory(inactive, used); err != nil {
		if !errors.Is(err, ErrInvalidImage) {
			return nil, err
		}
		// Apple can leave an unusable older generation. Preserve all existing bytes
		// for this update; publishing the selected graph repairs the other root.
		cursor = uint64((a.physicalSize-1)/a.geometry.chunk + 1)
	}
	return &chunkAllocation{
		used:      used,
		cursor:    cursor,
		highWater: a.physicalSize,
	}, nil
}

func (l *layer) markDirectory(directory map[int64]uint64, used map[uint64]struct{}) error {
	a := l.asif
	g := a.geometry
	tables := make(map[uint64]struct{})
	table := make([]byte, g.chunk)
	bitmap := make([]byte, g.chunk)
	for _, pointer := range directory {
		if _, ok := tables[pointer]; ok {
			continue
		}
		if l.asif.reservedEnd(pointer) != pointer {
			return invalid("table overlaps header or directory")
		}
		used[pointer] = struct{}{}
		if _, err := l.readTable(pointer, table); err != nil {
			return err
		}
		tables[pointer] = struct{}{}
		if err := l.markTable(table, bitmap, used); err != nil {
			return err
		}
	}
	return nil
}

func (l *layer) markTable(table, bitmap []byte, used map[uint64]struct{}) error {
	a := l.asif
	g := a.geometry
	for group := int64(0); group+g.groupBytes <= g.chunk; group += g.groupBytes {
		bitmapPointer := bigEndian.Uint64(table[group+g.entries*8:])
		if bitmapPointer != 0 {
			used[bitmapPointer] = struct{}{}
		}
		bitmapLoaded := false
		for entryIndex := int64(0); entryIndex < g.entries; entryIndex++ {
			entry := bigEndian.Uint64(table[group+entryIndex*8:])
			if entry & ^(physicalMask|3<<62) != 0 {
				return invalid("unknown chunk flags")
			}
			physical := entry & physicalMask
			chunkState := asifChunkState(entry >> 62)

			switch chunkState {
			case asifChunkInherited, asifChunkZero:
				if physical != 0 {
					used[physical] = struct{}{}
				}
			case asifChunkData:
				if physical != 0 {
					if err := l.markPayload(physical, used); err != nil {
						return err
					}
				}
			case asifChunkBitmap:
				if !bitmapLoaded {
					bitmapOffset, err := l.markChunk(bitmapPointer, g.chunk, used)
					if err != nil {
						return err
					}
					if _, err := l.io.ReadAt(bitmap, bitmapOffset); err != nil {
						return err
					}
					bitmapLoaded = true
				}
				if physical != 0 {
					if err := l.markPayload(physical, used); err != nil {
						return err
					}
				}
				if err := g.checkBitmap(bitmap, entryIndex); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (g geometry) checkBitmap(bitmap []byte, index int64) error {
	first := index * (g.chunk / g.block)
	for sector := int64(0); sector < g.chunk/g.block; sector++ {
		at := first + sector
		state := asifBlockState((bitmap[at/4] >> ((at % 4) * 2)) & 3)
		if state == asifBlockInvalid {
			return invalid("invalid bitmap state")
		}
	}
	return nil
}

func (l *layer) markPayload(pointer uint64, used map[uint64]struct{}) error {
	if l.asif.reservedEnd(pointer) != pointer {
		return invalid("payload overlaps header or directory")
	}
	if _, err := l.asif.physicalOffset(pointer); err != nil {
		return err
	}
	used[pointer] = struct{}{}
	return nil
}

func (l *layer) markChunk(pointer uint64, count int64, used map[uint64]struct{}) (int64, error) {
	if l.asif.reservedEnd(pointer) != pointer {
		return 0, invalid("physical chunk overlaps header or directory")
	}
	offset, err := l.asif.physicalRange(pointer, 0, count)
	if err != nil {
		return 0, err
	}
	used[pointer] = struct{}{}
	return offset, nil
}

func (u *asifUpdate) allocate(data []byte) (uint64, error) {
	allocation := u.allocation
	for {
		next := u.layer.asif.reservedEnd(allocation.cursor)
		if next != allocation.cursor {
			allocation.cursor = next
			continue
		}
		if _, used := allocation.used[allocation.cursor]; !used {
			break
		}
		allocation.cursor++
	}
	pointer := allocation.cursor
	chunk := u.layer.asif.geometry.chunk
	if pointer > uint64((math.MaxInt64-int64(len(data)))/chunk) {
		return 0, errors.New("diskimagekit: physical image is too large")
	}
	allocation.used[pointer] = struct{}{}
	allocation.cursor++
	offset := int64(pointer) * chunk
	if err := writeFullAt(u.layer.io, data, offset); err != nil {
		return 0, err
	}
	allocation.highWater = max(allocation.highWater, offset+int64(len(data)))
	return pointer, nil
}
