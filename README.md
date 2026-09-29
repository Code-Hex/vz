vz - Go binding with Apple [Virtualization.framework](https://developer.apple.com/documentation/virtualization?language=objc)
=======

[![Build](https://github.com/Code-Hex/vz/actions/workflows/compile.yml/badge.svg)](https://github.com/Code-Hex/vz/actions/workflows/compile.yml) [![Go Reference](https://pkg.go.dev/badge/github.com/Code-Hex/vz/v3.svg)](https://pkg.go.dev/github.com/Code-Hex/vz/v3)

vz provides the power of the Apple Virtualization.framework in Go. Put here is block quote of overreview which is written what is Virtualization.framework from the document.

> The Virtualization framework provides high-level APIs for creating and managing virtual machines (VM) on Apple silicon and Intel-based Mac computers. Use this framework to boot and run macOS or Linux-based operating systems in custom environments that you define. The framework supports the [Virtual I/O Device (VIRTIO)](https://docs.oasis-open.org/virtio/virtio/v1.1/csprd01/virtio-v1.1-csprd01.html) specification, which defines standard interfaces for many device types, including network, socket, serial port, storage, entropy, and memory-balloon devices.

## Usage

Please see the [example](https://github.com/Code-Hex/vz/tree/main/example) directory.

## Requirements

- macOS Monterey 12 or later with Go 1.25. Individual framework APIs may require a newer macOS version.
- Latest version of vz supports last two Go major [releases](https://go.dev/doc/devel/release) and might work with older versions.

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
- ✅ Native calls via purego with a precompiled Swift bridge

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

## Native bridge

Build with standard Go tooling and `CGO_ENABLED=1`. A small cgo adapter links the precompiled Swift archive, `libBridge.a`, into your executable on macOS arm64 and amd64. Calls use purego. Application builds require Clang and a macOS SDK, but do not compile Swift sources. The public Go API is unchanged. Runtime feature availability depends on both the host macOS version and the SDK used to generate the bridge.

Framework calls and native destruction run on the bridge dispatch queue. Go wrappers keep arguments alive across calls, and aliases share an owner until the last alias is collected. Asynchronous completions use request IDs without storing Go pointers in Swift. Window and UI operations still use Objective-C and must run on the main thread.

Signing your executable also covers the bridge code. There is no separate bridge dylib to extract, load, or sign. System frameworks and the Swift runtime stay dynamic and are loaded directly from macOS.

Go 1.25 requires macOS 12 or later. When targeting macOS 12 with a newer SDK, set the deployment target across both cgo compile flags and linker flags:

```sh
CGO_CFLAGS="-O2 -g -mmacosx-version-min=12.0" \
CGO_LDFLAGS="-O2 -g -mmacosx-version-min=12.0" \
CGO_ENABLED=1 go build .
```

### Rebuilding the bridge

After modifying native bridge sources or updating Xcode, run:

```sh
make generate/bridge
```

Regeneration requires Xcode with Swift 6.4 and a macOS SDK. The generator inspects Virtualization.framework's Swift symbol graph, resolves enum storage types with Clang, generates Swift wrappers, and compiles both arm64 and amd64 architectures under Swift 6 strict concurrency checks. It then emits static archives and Go bindings. The static archives bundle the compiler-selected Swift compatibility code, so application builds do not need to locate it in a Swift toolchain. Generated Swift is written to `internal/vzbridge/abi_arm64/Framework.swift` and `internal/vzbridge/abi_amd64/Framework.swift` for review.

Handwritten code in `internal/vzbridge/source` handles callbacks, delegates, KVO, resource lifetimes, and library-specific behavior. Pointer types declare ownership using `BorrowedObject`, `OwnedObject`, `CString`, `ErrorOut`, `RawPointer`, and `RawBytes`. The generator reads these declarations from the Swift compiler AST and verifies the generated C ABI. There are no separate JSON contracts; intermediate JSON remains in temporary directories.

The generator also produces a small `Virtualization.tbd` linker stub from verified weak imports. This supplies declarations missing from older SDKs to allow linking while preserving runtime availability checks. Application builds continue to link against the dynamic system framework.

Commit regenerated Swift files, Go bindings, `libBridge.a` static archives, and linker stubs alongside your source changes. ABI checks reject mismatched versions. If file replacement is interrupted, rerun generation.

### Adding framework APIs

List Swift declarations in the current SDK:

```sh
go run ./cmd/vzbridgegen -list
```

Regeneration scans every declaration in the SDK and generates bindings for all supported initializers, synchronous methods, factories, and property getters and setters. Public APIs do not need a selection list. You can call the generated bindings directly from the public Go API. Internal names combine the declaration path with a hash of its SDK identifier, so adding an overload does not rename an existing binding.

Parameter types, return types, and availability come from the SDK metadata, and unsupported declarations are reported with a reason. Asynchronous calls, actor-isolated declarations, collections, optional string inputs, and types without a supported C representation still require handwritten adapters. Classes with an external superclass other than `NSObject` also need an executor adapter. Inherited initializers absent from the symbol graph are not generated.

### Private APIs

List methods exposed by the running framework's Objective-C runtime:

```sh
go run ./cmd/vzbridgegen -list-private
```

Add the private methods you need to `cmd/vzbridgegen/private_selection.go`, then run `make generate/bridge`. Each entry specifies only the class, selector, and method kind. Only listed methods are generated as `UnsafePrivate_` bindings in the bundle. Missing, duplicate, or unsupported selections stop generation with an error.

Types come directly from the running framework's Objective-C metadata, so no `ipsw` installation is required. The `-list-private` flag lists all discovered methods, including those outside the selection. Private bindings reflect the host framework, which may differ from the installed SDK.

Private bindings remain raw calls and are not memory safe merely because of validation. Runtime metadata provides argument widths and signedness, but does not indicate ownership, consumed arguments, variadic arguments, or executor requirements. Verify those rules before calling a private binding. Object arguments and results use `unsafe.Pointer`; callers must keep Go owners alive through the call and arrange native releases. For an object result, pass `retainResult = true` to retain it before the bridge autorelease pool drains. Pass `false` only when the method already returns an owned reference or the object's lifetime is otherwise guaranteed. Initializers take a raw allocation from `UnsafePrivateAllocate`.

Generated calls validate the receiver, selector, return type, and every argument type on the target machine before invocation. A missing method or changed signature terminates the process unless an object-returning call receives an `invoked` output pointer. In that case, it returns nil and sets `invoked` to false, letting initializer callers release an allocation that was never consumed. A true value means the method ran, even if it returned nil. Runtime validation does not establish ownership or thread safety. Scalar values and raw pointers are supported. Swift-only private declarations, blocks, function pointers, and structs passed by value are not.

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

## Which projects use this library?

- [vfkit](https://github.com/crc-org/vfkit) is a macOS command-line hypervisor for Apple and Intel CPUs that supports most of Apple's Virtualization Framework features.
- [Lima](https://lima-vm.io/) launches Linux virtual machines with automatic file sharing and port forwarding (similar to WSL2).
- [linuxkit](https://github.com/linuxkit/linuxkit) is a toolkit for building custom minimal, immutable Linux distributions.

## LICENSE

MIT License
