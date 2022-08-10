vz - Go binding with Apple [Virtualization.framework](https://developer.apple.com/documentation/virtualization?language=objc)
=======

[![Build](https://github.com/Code-Hex/vz/actions/workflows/compile.yml/badge.svg)](https://github.com/Code-Hex/vz/actions/workflows/compile.yml)

vz provides the power of the Apple Virtualization.framework in Go. Put here is block quote of overreview which is written what is Virtualization.framework from the document.

> The Virtualization framework provides high-level APIs for creating and managing virtual machines on Apple silicon and Intel-based Mac computers. Use this framework to boot and run a Linux-based operating system in a custom environment that you define. The framework supports the Virtio specification, which defines standard interfaces for many device types, including network, socket, serial port, storage, entropy, and memory-balloon devices.

## USAGE

Please see the example directory.

## REQUIREMENTS

Higher or equal to Go 1.17.

### macOS Monterey (v12.x.x)

For the latest macOS version is developed on the [master](https://github.com/Code-Hex/vz) branch and released as `v2.x.x`.

You can install by `go get github.com/Code-Hex/vz/v2`

### macOS Big Sur (v11.x.x)

Some methods of this framework are available and some are not, depending on the version of macOS. Therefore, Go language side also needs to control which methods are available depending on the macOS version.

From now on, those available in Big Sur (11.0.0) will be developed on the [v1](https://github.com/Code-Hex/vz/tree/v1) branch and released as `v1.x.x`.

You can install by `go get github.com/Code-Hex/vz`

## IMPORTANT

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

## LICENSE

MIT License
