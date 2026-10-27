package diskimagekit_test

import (
	"io"

	"github.com/Code-Hex/vz/v4/diskimagekit"
)

// This example copies a stack without cache layers into an independent ASIF
// image. The source files remain read-only. It copies every logical byte, so
// large sparse images can take substantial time and storage.
func ExampleDiskImage() {
	source, err := diskimagekit.Open("base.asif", diskimagekit.WithOpenMode(diskimagekit.ReadOnly))
	if err != nil {
		panic(err)
	}
	defer source.Close()

	if err := source.Append("overlay.asif", diskimagekit.WithOpenMode(diskimagekit.ReadOnly)); err != nil {
		panic(err)
	}

	// Preserve the stack's block size, including when its layers differ.
	destination, err := diskimagekit.CreateASIF(
		"standalone.asif",
		source.BlockCount(),
		diskimagekit.WithBlockSize(source.BlockSize()),
	)
	if err != nil {
		panic(err)
	}
	defer destination.Close()

	reader := io.NewSectionReader(source, 0, source.Size())
	writer := io.NewOffsetWriter(destination, 0)
	buffer := make([]byte, 1<<20)
	n, err := io.CopyBuffer(writer, reader, buffer)
	if err != nil {
		panic(err)
	}
	if n != source.Size() {
		panic(io.ErrUnexpectedEOF)
	}
	if err := destination.Sync(); err != nil {
		panic(err)
	}

	// Release the destination's lock before opening it through a native disk
	// attachment, such as vz.NewDiskImageStorageDeviceAttachment.
	if err := destination.Close(); err != nil {
		panic(err)
	}
}
