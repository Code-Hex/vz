// Package diskimagekit reads and writes RAW and ASIF disk images and layered
// images. Its image, overlay, and cache concepts follow Apple's DiskImageKit.
// It does not call the Apple framework or create native DiskImage objects.
//
// Images hold advisory file locks until Close. All methods on an image may be
// called concurrently. Callers must close images when finished. No background
// goroutines or finalizers manage their lifetime.
//
// ASIF support covers version 1 images with 512 or 4096 byte blocks and chunks
// up to 64 MiB. New images use 1 MiB chunks. Writes copy changed data and tables,
// reusing storage that neither directory generation references. This limits
// growth during repeated overwrites but does not shrink the physical file.
// Sync reports storage errors but does not promise atomic recovery from power
// loss or torn sector writes.
//
// See https://developer.apple.com/documentation/diskimagekit.
package diskimagekit

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
)

// Format identifies an image's storage format.
//
// See https://developer.apple.com/documentation/diskimagekit/diskimage/format-swift.enum.
type Format uint8

const (
	// RAW is an unstructured image with 512 byte blocks.
	RAW Format = iota + 1
	// ASIF is an Apple Sparse Image Format image.
	ASIF
	// Stack is a base image with one or more ASIF layers.
	Stack
)

// BlockSize is the number of bytes in a logical block.
//
// See https://developer.apple.com/documentation/diskimagekit/diskimage/blocksize-swift.enum.
type BlockSize uint16

const (
	// BlockSize512 selects 512 byte blocks.
	BlockSize512 BlockSize = 512
	// BlockSize4096 selects 4096 byte blocks.
	BlockSize4096 BlockSize = 4096
)

// OpenMode controls file access. The default is Automatic.
//
// See https://developer.apple.com/documentation/diskimagekit/openconfiguration/mode-swift.enum.
type OpenMode uint8

const (
	// ReadOnly opens an image for reading and holds a shared advisory lock.
	ReadOnly OpenMode = iota
	// ReadWrite opens an image for writing and holds an exclusive advisory lock.
	ReadWrite
	// Automatic tries ReadWrite, falling back to ReadOnly only for permission
	// errors. Lock contention never causes a fallback.
	Automatic
)

// LayerType is the role of a layer in a stack. Zero means a standalone image
// or the base of a stack.
//
// See https://developer.apple.com/documentation/diskimagekit/diskimage/layertype-swift.struct.
type LayerType uint8

const (
	// Overlay stores changes without modifying its parent.
	Overlay LayerType = iota + 1
	// Cache stores copies of data read from its parent. A stack may have one cache.
	Cache
)

var (
	// ErrReadOnly means the requested operation requires writable storage.
	ErrReadOnly = errors.New("diskimagekit: image is read-only")
	// ErrInvalidImage means the image has malformed or unsupported ASIF data.
	ErrInvalidImage = errors.New("diskimagekit: invalid or unsupported image")
	// ErrIncompatibleStack means layers cannot form a valid stack.
	ErrIncompatibleStack = errors.New("diskimagekit: incompatible stack")
)

// LayerInfo is a snapshot of a file owned by a DiskImage.
type LayerInfo struct {
	// Path is the path used to open the file.
	Path string
	// Format is RAW or ASIF.
	Format Format
	// BlockCount is the logical size in blocks.
	BlockCount int64
	// BlockSize is the number of bytes in a logical block.
	//
	// See https://developer.apple.com/documentation/diskimagekit/diskimage/blocksize-swift.enum.
	BlockSize BlockSize
	// OpenMode is the access mode actually obtained.
	OpenMode OpenMode
	// LayerType is Overlay, Cache, or zero for a standalone image or stack base.
	LayerType LayerType
	// LayerUUID identifies this layer. It is empty when the image has no UUID.
	LayerUUID string
	// ParentUUID identifies the required parent. It is empty for RAW parents.
	ParentUUID string
}

// DiskImage owns a base file and its layers, ordered from base to top.
// Use Open, CreateRAW, or CreateASIF to construct one. A zero DiskImage is closed.
// ReadAt and WriteAt use byte offsets; creation and Truncate use block counts.
// A DiskImage must not be copied after first use.
//
// See https://developer.apple.com/documentation/diskimagekit/diskimage.
type DiskImage struct {
	mu     sync.Mutex
	layers []*layer
	closed bool
}

type layer struct {
	file        *os.File
	io          imageIO
	info        LayerInfo
	size        int64
	asif        *asifImage
	mutationErr error
}

// imageIO is the storage boundary used by ASIF updates. The file still owns
// locking and closing, including when a test injects a failed storage operation.
type imageIO interface {
	ReadAt([]byte, int64) (int, error)
	WriteAt([]byte, int64) (int, error)
	Sync() error
}

type openOptions struct {
	mode OpenMode
}
type createOptions struct {
	block BlockSize
}
type layerOptions struct {
	blocks *int64
}

// OpenOption configures Open or Append.
type OpenOption func(*openOptions) error

// WithOpenMode sets file access for Open or Append.
func WithOpenMode(mode OpenMode) OpenOption {
	return func(o *openOptions) error {
		if mode > Automatic {
			return fmt.Errorf("diskimagekit: invalid open mode %d", mode)
		}
		o.mode = mode
		return nil
	}
}

// CreateOption configures CreateASIF.
type CreateOption func(*createOptions) error

// WithBlockSize sets the logical block size. The default is BlockSize512.
func WithBlockSize(block BlockSize) CreateOption {
	return func(o *createOptions) error {
		if block != BlockSize512 && block != BlockSize4096 {
			return fmt.Errorf("diskimagekit: invalid block size %d", block)
		}
		o.block = block
		return nil
	}
}

// LayerOption configures CreateLayer.
type LayerOption func(*layerOptions) error

// WithBlockCount sets an overlay's size in blocks of its parent's block size.
// Without it, the overlay has the same logical size as its parent.
// Cache layers always have their parent's size and reject this option.
func WithBlockCount(count int64) LayerOption {
	return func(o *layerOptions) error {
		if count <= 0 {
			return errors.New("diskimagekit: block count must be positive")
		}
		o.blocks = &count
		return nil
	}
}

func parseOpenOptions(options []OpenOption) (OpenMode, error) {
	o := openOptions{mode: Automatic}
	for _, option := range options {
		if option == nil {
			return 0, errors.New("diskimagekit: nil open option")
		}
		if err := option(&o); err != nil {
			return 0, err
		}
	}
	return o.mode, nil
}

// Open opens a RAW or ASIF image. The default mode is Automatic.
// It holds a nonblocking advisory lock until Close. Format detection follows
// DiskImageKit: unrecognized ASIF format flags, segment counts, or metadata
// flags cause RAW detection. Invalid headers return ErrInvalidImage. Directory
// and data layout checks are deferred until an operation needs their mappings.
//
// See https://developer.apple.com/documentation/diskimagekit/diskimage.
func Open(path string, options ...OpenOption) (*DiskImage, error) {
	mode, err := parseOpenOptions(options)
	if err != nil {
		return nil, err
	}
	l, err := openLayer(path, mode)
	if err != nil {
		return nil, err
	}
	return &DiskImage{layers: []*layer{l}}, nil
}

func openLayer(path string, mode OpenMode) (*layer, error) {
	flags := os.O_RDONLY
	if mode != ReadOnly {
		flags = os.O_RDWR
	}
	f, err := openRegularFile(path, flags)
	if mode == Automatic {
		mode = ReadWrite
		if errors.Is(err, os.ErrPermission) {
			f, err = openRegularFile(path, os.O_RDONLY)
			mode = ReadOnly
		}
	}
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*layer, error) {
		f.Close()
		return nil, err
	}
	if err := lockFile(f, mode); err != nil {
		return fail(err)
	}
	stat, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	l := &layer{
		file: f,
		io:   f,
		size: stat.Size(),
		info: LayerInfo{
			Path:       path,
			Format:     RAW,
			BlockCount: stat.Size() / 512,
			BlockSize:  BlockSize512,
			OpenMode:   mode,
		},
	}
	var header [0x68]byte
	n, err := f.ReadAt(header[:], 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return fail(err)
	}
	if isASIF(header[:n]) {
		if err := l.parseASIF(); err != nil {
			return fail(err)
		}
	} else if stat.Size()%512 != 0 {
		return fail(fmt.Errorf("%w: RAW length is not a multiple of 512", ErrInvalidImage))
	}
	return l, nil
}

// CreateRAW creates a writable RAW image with 512 byte blocks. It replaces an
// existing file or symbolic link without modifying its old inode or link target.
// A dangling symbolic link is kept and its target is created instead.
// Failed creation leaves an existing file unchanged.
func CreateRAW(path string, blockCount int64) (*DiskImage, error) {
	size, err := byteSize(blockCount, BlockSize512)
	if err != nil {
		return nil, err
	}
	f, err := createFile(path, func(f *os.File) error {
		return f.Truncate(size)
	})
	if err != nil {
		return nil, err
	}
	l := &layer{
		file: f,
		io:   f,
		size: size,
		info: LayerInfo{
			Path:       path,
			Format:     RAW,
			BlockCount: blockCount,
			BlockSize:  BlockSize512,
			OpenMode:   ReadWrite,
		},
	}
	return &DiskImage{layers: []*layer{l}}, nil
}

// CreateASIF creates a writable ASIF v1 image. blockCount is in
// logical blocks, not bytes. WithBlockSize defaults to BlockSize512.
// It replaces an existing file or symbolic link without modifying its old inode
// or link target. A dangling symbolic link is kept and its target is created
// instead. Failed creation leaves an existing file unchanged.
func CreateASIF(path string, blockCount int64, options ...CreateOption) (*DiskImage, error) {
	o := createOptions{block: BlockSize512}
	for _, option := range options {
		if option == nil {
			return nil, errors.New("diskimagekit: nil create option")
		}
		if err := option(&o); err != nil {
			return nil, err
		}
	}
	l, err := createASIF(path, blockCount, o.block, 0, "")
	if err != nil {
		return nil, err
	}
	return &DiskImage{layers: []*layer{l}}, nil
}

func createFile(path string, initialize func(*os.File) error) (*os.File, error) {
	dir, name := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	path = dir + string(os.PathSeparator) + name
	info, err := os.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if errors.Is(err, os.ErrNotExist) {
		if target, err := os.Readlink(path); err == nil {
			if !filepath.IsAbs(target) {
				target = dir + string(os.PathSeparator) + target
			}
			return createFile(target, initialize)
		}
	}
	if err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New("diskimagekit: image must be a regular file")
		}
		existing, err := openRegularFile(path, os.O_WRONLY)
		if err != nil {
			return nil, err
		}
		if err := existing.Close(); err != nil {
			return nil, err
		}
	}
	temporary := filepath.Join(dir, ".diskimage-"+rand.Text())
	f, err := os.OpenFile(temporary, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	if err := lockFile(f, ReadWrite); err != nil {
		f.Close()
		return nil, err
	}
	if err := initialize(f); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return nil, err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func byteSize(blocks int64, block BlockSize) (int64, error) {
	if blocks <= 0 || blocks > math.MaxInt64/int64(block) {
		return 0, errors.New("diskimagekit: invalid block count")
	}
	return blocks * int64(block), nil
}

func (d *DiskImage) top() (*layer, error) {
	if d.closed || len(d.layers) == 0 {
		return nil, os.ErrClosed
	}
	return d.layers[len(d.layers)-1], nil
}

// Append opens an ASIF layer and adds it above the current top. Failure leaves
// the existing stack usable. Parent UUIDs must match the nearest non-cache
// layer when that layer has a UUID. A standalone cache or an image with a parent
// UUID cannot serve as the base of a stack.
//
// See https://developer.apple.com/documentation/diskimagekit/diskimage.
func (d *DiskImage) Append(path string, options ...OpenOption) error {
	mode, err := parseOpenOptions(options)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.top(); err != nil {
		return err
	}
	if err := d.checkBase(); err != nil {
		return err
	}
	l, err := openLayer(path, mode)
	if err != nil {
		return err
	}
	if err := d.checkLayer(l); err != nil {
		l.file.Close()
		return err
	}
	d.layers = append(d.layers, l)
	return nil
}

func (d *DiskImage) checkBase() error {
	if len(d.layers) != 1 {
		return nil
	}
	base := d.layers[0].info
	if base.LayerType == Cache || base.ParentUUID != "" {
		return fmt.Errorf("%w: base image requires a parent", ErrIncompatibleStack)
	}
	return nil
}

func (d *DiskImage) parentUUID() string {
	for i := len(d.layers) - 1; i >= 0; i-- {
		if d.layers[i].info.LayerType != Cache {
			return d.layers[i].info.LayerUUID
		}
	}
	return ""
}

func (d *DiskImage) checkLayer(l *layer) error {
	parent := d.layers[len(d.layers)-1]
	if l.asif == nil {
		return fmt.Errorf("%w: appended layer must be ASIF", ErrIncompatibleStack)
	}
	if l.info.LayerUUID == "" {
		return fmt.Errorf("%w: appended layer has no UUID", ErrIncompatibleStack)
	}
	if parent.mutationErr != nil {
		return parent.mutationErr
	}
	stat, err := l.file.Stat()
	if err != nil {
		return err
	}
	for _, existing := range d.layers {
		prior, err := existing.file.Stat()
		if err != nil {
			return err
		}
		if os.SameFile(stat, prior) {
			return fmt.Errorf("%w: repeated file", ErrIncompatibleStack)
		}
		if existing.info.LayerType == Cache && l.info.LayerType == Cache {
			return fmt.Errorf("%w: multiple cache layers", ErrIncompatibleStack)
		}
	}
	parentID := d.parentUUID()
	if parentID != "" && l.info.ParentUUID != parentID {
		return fmt.Errorf("%w: parent UUID mismatch", ErrIncompatibleStack)
	}
	if l.info.LayerType != Cache {
		l.info.LayerType = Overlay
	}
	return nil
}

// CreateLayer creates an ASIF overlay or cache above the current image.
// It replaces an existing file or symbolic link. On failure the previous stack
// and an existing file at path remain unchanged. The layer inherits its
// parent's block size. The destination must not refer to a file already in the
// stack, including through a hard link or symbolic link. A stack may contain
// one cache.
//
// See https://developer.apple.com/documentation/diskimagekit/diskimage.
func (d *DiskImage) CreateLayer(path string, kind LayerType, options ...LayerOption) error {
	if kind != Overlay && kind != Cache {
		return errors.New("diskimagekit: invalid layer type")
	}
	o := layerOptions{}
	for _, option := range options {
		if option == nil {
			return errors.New("diskimagekit: nil layer option")
		}
		if err := option(&o); err != nil {
			return err
		}
	}
	if kind == Cache && o.blocks != nil {
		return errors.New("diskimagekit: cache size is inherited")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	parent, err := d.top()
	if err != nil {
		return err
	}
	if parent.mutationErr != nil {
		return parent.mutationErr
	}
	if err := d.checkBase(); err != nil {
		return err
	}
	if kind == Cache {
		for _, l := range d.layers {
			if l.info.LayerType == Cache {
				return fmt.Errorf("%w: multiple cache layers", ErrIncompatibleStack)
			}
		}
	}
	destination, err := os.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		for _, l := range d.layers {
			owned, err := l.file.Stat()
			if err != nil {
				return err
			}
			if os.SameFile(destination, owned) {
				return fmt.Errorf("%w: layer path refers to a file already in the stack", ErrIncompatibleStack)
			}
		}
	}
	blocks := parent.info.BlockCount
	if o.blocks != nil {
		blocks = *o.blocks
	}
	l, err := createASIF(path, blocks, parent.info.BlockSize, kind, d.parentUUID())
	if err != nil {
		return err
	}
	d.layers = append(d.layers, l)
	return nil
}

// ReadAt reads logical bytes from the stack. Unallocated data inherits from
// lower layers; explicit zero mappings hide lower data. A cache miss fills a
// whole chunk in writable cache layers. A miss in a read-only cache returns
// ErrReadOnly. Cache write errors are returned.
// Inherited data in a detached cache or an ASIF image with a parent UUID
// requires its parent and returns ErrIncompatibleStack. An overlay on a RAW
// parent cannot be distinguished from a standalone base by its header alone.
func (d *DiskImage) ReadAt(p []byte, off int64) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	top, err := d.top()
	if err != nil {
		return 0, err
	}
	if off < 0 {
		return 0, errors.New("diskimagekit: negative read offset")
	}
	if len(p) == 0 {
		return 0, nil
	}
	if off >= top.size {
		return 0, io.EOF
	}
	count := min(int64(len(p)), top.size-off)
	n, err := d.read(len(d.layers)-1, p[:count], off, true)
	if err == nil && n < len(p) {
		err = io.EOF
	}
	return n, err
}

func (d *DiskImage) read(index int, p []byte, off int64, fill bool) (int, error) {
	if index < 0 {
		clear(p)
		return len(p), nil
	}
	l := d.layers[index]
	n := 0
	for len(p) > 0 {
		if off >= l.size {
			clear(p)
			return n + len(p), nil
		}
		e, err := l.extent(off)
		if err != nil {
			return n, err
		}
		count := min(int64(len(p)), e.length, l.size-off)
		part := p[:count]
		switch e.kind {
		case extentData:
			var read int
			var err error
			if l.asif == nil {
				read, err = l.io.ReadAt(part, e.offset)
			} else {
				read, err = l.readPayload(part, e.offset)
			}
			if err != nil {
				return n + read, err
			}
		case extentZero:
			clear(part)
		case extentInherited:
			if err := d.requireParent(index); err != nil {
				return n, err
			}
			if fill && l.info.LayerType == Cache {
				if err := d.fillCache(index, off); err != nil {
					return n, err
				}
				continue
			}
			if read, err := d.read(index-1, part, off, fill); err != nil {
				return n + read, err
			}
		}
		n += int(count)
		off += count
		p = p[count:]
	}
	return n, nil
}

func (d *DiskImage) requireParent(index int) error {
	l := d.layers[index]
	if index == 0 && (l.info.LayerType == Cache || l.info.ParentUUID != "") {
		return fmt.Errorf("%w: inherited data requires the parent image", ErrIncompatibleStack)
	}
	return nil
}

// WriteAt writes logical bytes into the top image without changing lower layers.
// A write cannot extend the logical size. Writes crossing the end return the
// number written and io.ErrShortWrite. A cache cannot be the writable top.
// ASIF metadata failures disable further mutations on this handle; close and
// reopen it before deciding whether to retry. Reads remain available.
func (d *DiskImage) WriteAt(p []byte, off int64) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	l, err := d.top()
	if err != nil {
		return 0, err
	}
	if err := l.writable(); err != nil {
		return 0, err
	}
	if l.info.LayerType == Cache {
		return 0, errors.New("diskimagekit: cannot write to a cache")
	}
	if off < 0 {
		return 0, errors.New("diskimagekit: negative write offset")
	}
	if len(p) == 0 {
		return 0, nil
	}
	if off >= l.size {
		return 0, io.ErrShortWrite
	}
	count := min(int64(len(p)), l.size-off)
	if l.asif == nil {
		n, err := l.io.WriteAt(p[:count], off)
		if err == nil && n < len(p) {
			err = io.ErrShortWrite
		}
		return n, err
	}
	if err := d.writeASIF(len(d.layers)-1, p[:count], off); err != nil {
		return 0, err
	}
	if count < int64(len(p)) {
		return int(count), io.ErrShortWrite
	}
	return int(count), nil
}

func (l *layer) writable() error {
	if l.info.OpenMode != ReadWrite || (l.asif != nil && l.asif.readOnly) {
		return ErrReadOnly
	}
	return l.mutationErr
}

// Truncate changes the logical size in blocks, not bytes. An overlay expanding
// into an unallocated range inherits its parent. ASIF shrinking visits existing
// mapping tables only. Whole discarded chunks become zero; an inherited partial
// chunk and absent tables keep inheriting when regrown. If a partial chunk has
// no owned data left, the whole chunk takes its first block's mapping state.
// Cache layers cannot be resized. An unchanged size succeeds even when the
// image is read-only.
//
// See https://developer.apple.com/documentation/diskimagekit/diskimage/truncate(blockcount:).
func (d *DiskImage) Truncate(blockCount int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	l, err := d.top()
	if err != nil {
		return err
	}
	if l.mutationErr != nil {
		return l.mutationErr
	}
	if l.info.LayerType == Cache {
		return errors.New("diskimagekit: cannot truncate a cache")
	}
	size, err := byteSize(blockCount, l.info.BlockSize)
	if err != nil {
		return err
	}
	if size == l.size {
		return nil
	}
	if err := l.writable(); err != nil {
		return err
	}
	if l.asif != nil {
		return d.truncateASIF(size)
	}
	if err := l.file.Truncate(size); err != nil {
		return err
	}
	l.size = size
	l.info.BlockCount = blockCount
	return nil
}

// Sync flushes every writable file. An earlier ASIF mutation failure is returned
// rather than retried. Sync does not make the stack a power-fail transaction.
func (d *DiskImage) Sync() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.top(); err != nil {
		return err
	}
	for _, l := range d.layers {
		if l.mutationErr != nil {
			return l.mutationErr
		}
		if l.info.OpenMode == ReadWrite {
			if err := l.io.Sync(); err != nil {
				if l.asif != nil {
					l.mutationErr = err
				}
				return err
			}
		}
	}
	return nil
}

// Close closes every owned file and releases its advisory lock. It waits for
// concurrent operations on this image to finish. Repeated calls return nil.
// Close does not replace an explicit Sync and does not report successful flushes.
func (d *DiskImage) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	var err error
	for i := len(d.layers) - 1; i >= 0; i-- {
		err = errors.Join(err, d.layers[i].file.Close())
	}
	return err
}

// Layers returns independent snapshots ordered from base to top.
func (d *DiskImage) Layers() []LayerInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]LayerInfo, len(d.layers))
	for i, l := range d.layers {
		result[i] = l.info
	}
	if len(result) == 1 {
		result[0].LayerType = 0
	}
	return result
}

func (d *DiskImage) info() LayerInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.layers) == 0 {
		return LayerInfo{}
	}
	return d.layers[len(d.layers)-1].info
}

// Size returns the logical size in bytes, including after Close.
func (d *DiskImage) Size() int64 {
	info := d.info()
	return info.BlockCount * int64(info.BlockSize)
}

// BlockCount returns the logical block count, including after Close.
//
// See https://developer.apple.com/documentation/diskimagekit/diskimage/blockcount.
func (d *DiskImage) BlockCount() int64 {
	return d.info().BlockCount
}

// BlockSize returns the top layer's logical block size.
//
// See https://developer.apple.com/documentation/diskimagekit/diskimage/blocksize-swift.property.
func (d *DiskImage) BlockSize() BlockSize {
	return d.info().BlockSize
}

// OpenMode returns the image's access mode. A stack with a cache on top is
// read-only even when the cache file is writable for filling. A standalone
// image reports its file access mode, as does each entry in Layers.
// ASIF header restrictions can also prevent writes after a ReadWrite open.
//
// See https://developer.apple.com/documentation/diskimagekit/diskimage/openmode.
func (d *DiskImage) OpenMode() OpenMode {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.layers) == 0 {
		return ReadOnly
	}
	info := d.layers[len(d.layers)-1].info
	if len(d.layers) > 1 && info.LayerType == Cache {
		return ReadOnly
	}
	return info.OpenMode
}

// LayerUUID returns the top layer's UUID, or an empty string when none is set.
// An ASIF layer gets a new UUID before its first write through a writable open.
// Cache UUIDs do not change during filling.
//
// See https://developer.apple.com/documentation/diskimagekit/diskimage/layeruuid.
func (d *DiskImage) LayerUUID() string {
	return d.info().LayerUUID
}

// ParentUUID returns the top layer's required parent UUID, if any.
//
// See https://developer.apple.com/documentation/diskimagekit/diskimage/parentuuid.
func (d *DiskImage) ParentUUID() string {
	return d.info().ParentUUID
}

// Format returns Stack for layered images, or the standalone file format.
//
// See https://developer.apple.com/documentation/diskimagekit/diskimage/format-swift.property.
func (d *DiskImage) Format() Format {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.layers) > 1 {
		return Stack
	}
	if len(d.layers) == 0 {
		return 0
	}
	return d.layers[0].info.Format
}
