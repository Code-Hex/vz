//go:build darwin && arm64

package vz

func wrapLinuxRosettaDirectoryShare(ptr *pointer) DirectoryShare {
	return &LinuxRosettaDirectoryShare{pointer: ptr}
}
