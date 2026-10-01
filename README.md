vz - Go binding with Apple [Virtualization.framework](https://developer.apple.com/documentation/virtualization?language=objc)
=======

[![Build](https://github.com/Code-Hex/vz/actions/workflows/compile.yml/badge.svg)](https://github.com/Code-Hex/vz/actions/workflows/compile.yml) [![Go Reference](https://pkg.go.dev/badge/github.com/Code-Hex/vz/v3.svg)](https://pkg.go.dev/github.com/Code-Hex/vz/v3)

vz provides the power of the Apple Virtualization.framework in Go. Put here is block quote of overreview which is written what is Virtualization.framework from the document.

> The Virtualization framework provides high-level APIs for creating and managing virtual machines (VM) on Apple silicon and Intel-based Mac computers. Use this framework to boot and run macOS or Linux-based operating systems in custom environments that you define. The framework supports the [Virtual I/O Device (VIRTIO)](https://docs.oasis-open.org/virtio/virtio/v1.1/csprd01/virtio-v1.1-csprd01.html) specification, which defines standard interfaces for many device types, including network, socket, serial port, storage, entropy, and memory-balloon devices.

## Usage

Please see the [example](https://github.com/Code-Hex/vz/tree/main/example) directory.

## Requirements

- macOS Monterey (12.0) or later.
- Go 1.25 or later.
- Xcode or Command Line Tools, with `CGO_ENABLED=1`.

A normal `go build` compiles the Objective-C helpers and links Apple's frameworks.
The Go bindings use purego for API calls. Consumers do not need Swift, prebuilt bridge
binaries, or a generation step. `go mod vendor` builds from the same sources.

## Installation

Initialize your project by creating a folder and then running `go mod init github.com/your/repo` ([learn more](https://go.dev/blog/using-go-modules)) inside the folder. Then install vz with the go get command:

```
$ go get github.com/Code-Hex/vz/v3
```

Deprecated older versions (v1, v2).

## Feature Overview

- ✅ Virtualize Linux on a Mac **(x86_64, arm64)**
  - GUI Support
  - Boot Extensible Firmware Interface (EFI) ROM
  - Clipboard sharing through the SPICE agent
- ✅ Virtualize macOS on Apple Silicon Macs **(arm64)**
    - Fetches the latest restore image supported by this host from the network
  - Start in recovery mode
- ✅ Running Intel Binaries in Linux VMs with Rosetta **(arm64)**
- ✅ [Shared Directories](https://github.com/Code-Hex/vz/wiki/Shared-Directories)
- ✅ [Virtio Sockets](https://github.com/Code-Hex/vz/wiki/Sockets)

## Important

For binaries used in this package, you need to create an entitlements file like the one below and apply the following command.

<details>
<summary>vz.entitlements</summary>

```
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>com.apple.security.virtualization</key>
	<true/>
</dict>
</plist>
```

</details>

```sh
$ codesign --entitlements vz.entitlements -s - <YOUR BINARY PATH>
```

> A process must have the com.apple.security.virtualization entitlement to use the Virtualization APIs.

If you want to use [`VZBridgedNetworkDeviceAttachment`](https://developer.apple.com/documentation/virtualization/vzbridgednetworkdeviceattachment?language=objc), you need to add also `com.apple.vm.networking` entitlement.

## Build SDK

API availability depends on both the running macOS version and the build SDK.
A binary built with an older SDK can return `ErrBuildTargetOSVersion` for newer APIs.
Build with an SDK that includes the APIs you need.

## Version compatibility check

The package provides a mechanism for checking the availability of the respective API through error handling:

```go
bootLoader, err := vz.NewEFIBootLoader()
if errors.Is(err, vz.ErrUnsupportedOSVersion) || errors.Is(err, vz.ErrBuildTargetOSVersion) {
  return fallbackBootLoader()
}
if err != nil {
  return nil, err
}
return bootLoader, nil
```

There are two items to check.

1. API is compatible with the version of macOS
2. The binary was built with the API enabled

## Knowledge for the Apple Virtualization.framework

There is a lot of knowledge required to use this Apple Virtualization.framework, but the information is too scattered and very difficult to understand. In most cases, this can be found in [the official documentation](https://developer.apple.com/documentation/virtualization?language=objc). However, the Linux kernel knowledge required to use the feature provided by this framework is not documented. Therefore, I have compiled the knowledge I have gathered so far into this wiki.

https://github.com/Code-Hex/vz/wiki

Anyone is free to edit this wiki. It would help someone if you could add information not listed here. Let's make a good wiki together!

## Testing

If you want to contribute some code, you will need to add tests.

[PUI PUI Linux](https://github.com/Code-Hex/puipui-linux) is used to test this library. This Linux is designed to provide only the minimum functionality required for the Apple Virtualization.framework (Virtio), so the kernel file size is very small.

The test code uses the `Makefile` in the project root.

```
$ # Download PUI PUI Linux, Only required the first time.
$ make download_kernel
$ make test
```

The public Go API wraps bindings generated from SDK types, ownership attributes,
and availability declarations. To regenerate bindings with your installed SDK and
Virtualization runtime, run:

```sh
go generate ./internal/vzbridge
go test ./cmd/vzbridgegen ./internal/vzbridge/nativegen
make test/graphics
```

The generator discovers all methods in the Virtualization runtime image, including
private methods. Unsupported signatures remain listed with reasons in the generated
Go files. For detailed compiler and runtime facts, pass `-report /tmp/vzbridge.json`
to `go run ./cmd/vzbridgegen`. Reports are not generation inputs.

Private bindings are internal and unsafe. Runtime encodings do not describe
ownership or variadic arguments. Callers must establish those contracts. The
runtime scan describes the generating host, and every private call checks its
receiver and ABI before dispatch. Regenerate on another host to discover methods
specific to that host. Unused bindings have no startup registration.

## Which projects use this library?

- [vfkit](https://github.com/crc-org/vfkit) is a macOS command-line hypervisor for Apple and Intel CPUs that supports most of Apple's Virtualization Framework features.
- [Lima](https://lima-vm.io/) launches Linux virtual machines with automatic file sharing and port forwarding (similar to WSL2).
- [linuxkit](https://github.com/linuxkit/linuxkit) is a toolkit for building custom minimal, immutable Linux distributions.

## LICENSE

MIT License
