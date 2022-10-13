package vz

import (
	"testing"
)

func TestNonExistingFileSerialPortAttachment(t *testing.T) {
	_, err := NewFileSerialPortAttachment("/non/existing/path", false)
	if err == nil {
		t.Error("NewFileSerialPortAttachment should have returned an error")
	}
}
