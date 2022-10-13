package vz

import (
	"strconv"
	"strings"
	"sync"
	"syscall"
)

func macosMajorVersionLessThan(major int) bool {
	return macOSMajorVersion() < major
}

var (
	majorVersion     int
	majorVersionOnce interface{ Do(func()) } = &sync.Once{}
)

// This can be replaced in the test code to enable mock.
// It will not be changed in production.
var fetchMajorVersion = func() {
	osver, err := syscall.Sysctl("kern.osproductversion")
	if err != nil {
		panic(err)
	}
	osverArray := strings.Split(osver, ".")
	major, err := strconv.Atoi(osverArray[0])
	if err != nil {
		panic(err)
	}
	majorVersion = major
}

func macOSMajorVersion() int {
	majorVersionOnce.Do(fetchMajorVersion)
	return majorVersion
}
