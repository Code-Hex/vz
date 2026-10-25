package vz

import (
	"errors"
	"os"
	"runtime/cgo"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// GuestMemoryMapping exposes a guest RAM range in the host address space.
// Its memory becomes invalid when the guest reboots or shuts down. Keep the
// mapping alive while using MutableBytes. Guest writes can change the bytes at
// any time, so copy untrusted input once before validating and interpreting it.
//
// see: https://developer.apple.com/documentation/virtualization/vzguestmemorymapping?language=objc
type GuestMemoryMapping struct {
	*pointer
}

// MutableBytes returns the base pointer of the mapped memory.
//
// see: https://developer.apple.com/documentation/virtualization/vzguestmemorymapping/mutablebytes?language=objc
func (m *GuestMemoryMapping) MutableBytes() unsafe.Pointer {
	return vzbridge.VZGuestMemoryMapping_MutableBytes(m)
}

// PhysicalAddress returns the base guest physical address.
//
// see: https://developer.apple.com/documentation/virtualization/vzguestmemorymapping/physicaladdress?language=objc
func (m *GuestMemoryMapping) PhysicalAddress() uint64 {
	return vzbridge.VZGuestMemoryMapping_PhysicalAddress(m)
}

// Length returns the mapping length in bytes.
//
// see: https://developer.apple.com/documentation/virtualization/vzguestmemorymapping/length?language=objc
func (m *GuestMemoryMapping) Length() uint64 {
	return vzbridge.VZGuestMemoryMapping_Length(m)
}

// VirtioSharedMemoryRegion maps host memory into a configured guest region.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosharedmemoryregion?language=objc
type VirtioSharedMemoryRegion struct {
	*pointer
}

// RegionID returns the device-specific region identifier.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosharedmemoryregion/regionid?language=objc
func (r *VirtioSharedMemoryRegion) RegionID() uint8 {
	return vzbridge.VZVirtioSharedMemoryRegion_RegionID(r)
}

// Size returns the region size in bytes.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosharedmemoryregion/size?language=objc
func (r *VirtioSharedMemoryRegion) Size() uint64 {
	return vzbridge.VZVirtioSharedMemoryRegion_Size(r)
}

// MapMemory asynchronously maps host memory into the region.
// memory, offset, and size must be aligned to the host page size. memory must
// point to non-Go memory that remains allocated until unmapping completes.
// completion runs on the device queue and must not block it.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosharedmemoryregion/mapmemory(_:atoffset:size:completionhandler:)?language=objc
func (r *VirtioSharedMemoryRegion) MapMemory(memory unsafe.Pointer, offset, size uint64, completion func(error)) error {
	if completion == nil {
		return errors.New("completion must not be nil")
	}
	page := uint64(os.Getpagesize())
	if offset%page != 0 || size == 0 || size%page != 0 || offset > r.Size() || size > r.Size()-offset {
		return errors.New("mapping range must fit the region and be aligned to the host page size")
	}
	if memory == nil || uintptr(memory)%uintptr(page) != 0 {
		return errors.New("memory must be non-nil and aligned to the host page size")
	}
	handle := cgo.NewHandle(completion)
	vzbridge.MapCustomVirtioMemory(r, memory, offset, size, customVirtioCallback(), uint64(handle))
	return nil
}

// UnmapMemory asynchronously removes a mapping. offset and size must be aligned
// to the host page size. completion runs on the device queue.
//
// see: https://developer.apple.com/documentation/virtualization/vzvirtiosharedmemoryregion/unmapmemory(atoffset:size:completionhandler:)?language=objc
func (r *VirtioSharedMemoryRegion) UnmapMemory(offset, size uint64, completion func(error)) error {
	if completion == nil {
		return errors.New("completion must not be nil")
	}
	page := uint64(os.Getpagesize())
	if offset%page != 0 || size == 0 || size%page != 0 || offset > r.Size() || size > r.Size()-offset {
		return errors.New("mapping range must fit the region and be aligned to the host page size")
	}
	handle := cgo.NewHandle(completion)
	vzbridge.UnmapCustomVirtioMemory(r, offset, size, customVirtioCallback(), uint64(handle))
	return nil
}
