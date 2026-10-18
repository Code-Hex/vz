package main

import (
	"strings"
	"testing"
)

func TestPrivateQueueInheritanceBoundaries(t *testing.T) {
	for _, test := range []struct {
		owner    string
		selector string
		inherit  bool
	}{
		{"VZVirtioConsoleDevice", "ports", true},
		{"VZVirtioConsoleDeviceConfiguration", "ports", false},
		{"VZVirtioConsolePort", "attachment", false},
		{"VZCustomVirtioDevice", "sharedMemoryRegions", true},
		{"VZCustomVirtioDevice", "deviceQueue", false},
		{"VZVirtioQueue", "nextElement", true},
	} {
		t.Run(test.owner+"/"+test.selector, func(t *testing.T) {
			private, err := generatePrivateMethod(privateMethod{Class: test.owner, Selector: test.selector, Result: "@", Arguments: []string{"@", ":"}}, "Fixture")
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(private, "InheritObjectQueue(result,"); got != test.inherit {
				t.Errorf("private inheritance = %t, want %t", got, test.inherit)
			}
		})
	}
}
