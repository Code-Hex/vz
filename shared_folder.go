package vz

/*
#cgo darwin CFLAGS: -x objective-c -fno-objc-arc
#cgo darwin LDFLAGS: -lobjc -framework Foundation -framework Virtualization
# include "virtualization.h"
*/
import "C"
import "runtime"

type DirectorySharingDeviceConfiguration interface {
	NSObject

	directorySharingDeviceConfiguration()
}

type baseDirectorySharingDeviceConfiguration struct{}

func (*baseDirectorySharingDeviceConfiguration) directorySharingDeviceConfiguration() {}

var _ DirectorySharingDeviceConfiguration = (*VirtioFileSystemDeviceConfiguration)(nil)

type VirtioFileSystemDeviceConfiguration struct {
	pointer

	*baseDirectorySharingDeviceConfiguration
}

func NewVirtioFileSystemDeviceConfiguration(tag string) *VirtioFileSystemDeviceConfiguration {
	tagChar := charWithGoString(tag)
	defer tagChar.Free()
	fsdConfig := &VirtioFileSystemDeviceConfiguration{
		pointer: pointer{
			ptr: C.newVZVirtioFileSystemDeviceConfiguration(tagChar.CString()),
		},
	}
	runtime.SetFinalizer(fsdConfig, func(self *VirtioFileSystemDeviceConfiguration) {
		self.Release()
	})
	return fsdConfig
}

func (c *VirtioFileSystemDeviceConfiguration) SetDirectoryShare(share DirectoryShare) {
	C.setVZVirtioFileSystemDeviceConfigurationShare(c.Ptr(), share.Ptr())
}

type DirectoryShare interface {
	NSObject

	directoryShare()
}

type baseDirectoryShare struct{}

func (*baseDirectoryShare) directoryShare() {}

var _ DirectoryShare = (*SingleDirectoryShare)(nil)

type SharedDirectory struct {
	pointer
}

func NewSharedDirectory(dirPath string, readOnly bool) *SharedDirectory {
	dirPathChar := charWithGoString(dirPath)
	defer dirPathChar.Free()
	sd := &SharedDirectory{
		pointer: pointer{
			ptr: C.newVZSharedDirectory(dirPathChar.CString(), C.bool(readOnly)),
		},
	}
	runtime.SetFinalizer(sd, func(self *SharedDirectory) {
		self.Release()
	})
	return sd
}

type SingleDirectoryShare struct {
	pointer

	*baseDirectoryShare
}

func NewSingleDirectoryShare(share *SharedDirectory) *SingleDirectoryShare {
	config := &SingleDirectoryShare{
		pointer: pointer{
			ptr: C.newVZSingleDirectoryShare(share.Ptr()),
		},
	}
	runtime.SetFinalizer(config, func(self *SingleDirectoryShare) {
		self.Release()
	})
	return config
}
