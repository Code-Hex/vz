package vz

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNonExistingFileSerialPortAttachment(t *testing.T) {
	_, err := NewFileSerialPortAttachment("/non/existing/path", false)
	require.Error(t, err)
}
