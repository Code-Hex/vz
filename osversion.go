package vz

import (
	"strconv"
	"strings"
	"sync"
	"syscall"
)

func macosMajorVersionLessThan(major float64) bool {
	return macOSMajorVersion() < major
}

var (
	majorVersion     float64
	majorVersionOnce interface{ Do(func()) } = &sync.Once{}

	// This can be replaced in the test code to enable mock.
	// It will not be changed in production.
	sysctl = syscall.Sysctl
)

func fetchMajorVersion() (float64, error) {
	osver, err := sysctl("kern.osproductversion")
	if err != nil {
		return 0, err
	}
	osverArray := strings.SplitAfterN(osver, ".", 1)
	version, err := strconv.ParseFloat(osverArray[0], 64)
	if err != nil {
		return 0, err
	}
	return version, nil
}

func macOSMajorVersion() float64 {
	majorVersionOnce.Do(func() {
		version, err := fetchMajorVersion()
		if err != nil {
			panic(err)
		}
		majorVersion = version
	})
	return majorVersion
}
