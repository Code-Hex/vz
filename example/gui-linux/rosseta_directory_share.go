//go:build darwin && !arm64
// +build darwin,!arm64

package main

import "github.com/Code-Hex/vz/v3"

func createRosettaDirectoryShareConfiguration() (*vz.VirtioFileSystemDeviceConfiguration, error) {
	return nil, errIgnoreInstall
}
