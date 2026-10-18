# Connect vmnet to a VM through a socket

`vmnetsock` connects a `vmnet.Interface` to a VM through a Unix socket pair. It forwards Ethernet frames between the interface and the socket. `Adapter.Dup` returns a socket descriptor as an `*os.File` for the caller to pass to the VM.

The interface can belong to a `vmnet.Network` or bridge directly to a physical interface. Both require macOS 26 or newer and a compatible SDK.

`vmnetsock.New` supports two socket protocols:

* `vmnetsock.Stream` uses QEMU's stream and socket protocol, a four-byte big-endian length followed by an Ethernet frame.
* `vmnetsock.Datagram` uses krunkit's unixgram protocol, with one Ethernet frame per datagram.

## Usage example

```go
import "github.com/Code-Hex/vz/v4/vmnetsock"
```

```go
configuration, err := vmnet.NewNetworkConfiguration(vmnet.ModeShared)
if err != nil {
	return err
}
network, err := vmnet.NewNetwork(configuration)
if err != nil {
	return err
}
iface, err := vmnet.StartInterfaceWithNetwork(network)
if err != nil {
	return err
}
defer iface.Stop()
adapter, err := vmnetsock.New(ctx, iface, vmnetsock.Stream)
if err != nil {
	return err
}
defer adapter.Close()
file, err := adapter.Dup()
if err != nil {
	return err
}
defer file.Close()
cmd := exec.CommandContext(ctx, "qemu-system-aarch64",
	"-netdev", "stream,id=net0,addr.type=fd,addr.str=3",
	"-device", "virtio-net-pci,netdev=net0,mac="+iface.MACAddress().String())
cmd.ExtraFiles = []*os.File{file}
runErr := cmd.Run()
return errors.Join(runErr, adapter.Close())
```

## Bridge to a physical interface

`vmnet.BridgedInterfaces()` lists the physical interfaces eligible for bridged mode. Use a name from that list instead of creating a `Network`:

```go
iface, err := vmnet.StartBridgedInterface("en0")
if err != nil {
	return err
}
defer iface.Stop()
```

Pass this interface to `vmnetsock.New` as in the example above. `StartBridgedInterface` accepts the same `InterfaceOption` values as `StartInterfaceWithNetwork`.

Bridged mode connects the guest to the physical network. DHCP, DNS, and routing depend on the physical network configuration. No vmnet `NetworkConfiguration` is created. The selected interface must still be available when the interface starts.

Bridged mode uses `vmnet_start_interface`. Access depends on the process's privileges and entitlements. Apple documents [`com.apple.vm.networking`](https://developer.apple.com/documentation/vmnet) as required for sandboxed processes that use vmnet. This restricted entitlement must be authorized by a provisioning profile, as explained in [Apple's signing guidance](https://developer.apple.com/forums/thread/850614).

## Pass the descriptor to the VM

Add the network and device flags to your existing virtual machine configuration. The first file passed to `exec.Cmd.ExtraFiles` maps to file descriptor 3 in the child process.

Stream also supports QEMU's older `-netdev socket,id=net0,fd=3` syntax. For krunkit, select `vmnetsock.Datagram` and pass the descriptor and MAC address through `--device virtio-net,type=unixgram,fd=3,mac=...`.

The `Adapter` owns both ends of the socket pair. Each `Adapter.Dup` call returns a caller-owned `*os.File` duplicate of the peer socket, with close-on-exec set. Use `defer file.Close()` to close the duplicate when your function returns. The example keeps it open until the VM exits.

Closing a duplicate does not stop forwarding. When the VM exits, call `Adapter.Close` to stop forwarding and wait for cleanup. The example returns the process error and the forwarding error with `errors.Join`. Deferred calls cover early returns; calling `Adapter.Close` again is safe.

`Dup` returns `os.ErrClosed` after the adapter has closed. `Adapter.Close` never closes caller-owned duplicates or stops the borrowed `Interface`.

## Stop forwarding

`Adapter.Close` stops packet forwarding, closes its own sockets, and waits for both forwarding goroutines to exit. It returns the first cause of termination:

* A peer EOF in Stream mode or an explicit call to `Close` returns `nil` if it triggers termination first.
* If a forwarding failure or context cancellation occurs first, subsequent calls to `Close` preserve that original error.
* Context cancellation returns `context.Canceled` or `context.DeadlineExceeded`.

The adapter retains its peer socket until forwarding stops. Closing all duplicates does not end forwarding in either transport. Cancel the context or call `Adapter.Close` when the VM exits.

Multiple readers on one `Interface` divide the received packets, so normally attach one `Adapter` per `Interface`.

## Interface lifetime and options

All copies of an `Interface` share a single lifetime. Calling `Stop` invalidates the interface; subsequent reads and writes return `net.ErrClosed`.

The SDK reference from an `Interface` to its `Network` is independent of your application's `Network` reference. Calling `Stop` on an `Interface` does not terminate the underlying `Network`, and you can start a new `Interface` on that same `Network` afterward.

On a startup error, both constructors return a nil `Interface` after attempting cleanup. The returned error includes any cleanup failure. `Stop` releases local resources even if the native stop fails. Repeated calls return the original result without retrying.

With no options, both constructors enable MAC allocation and disable offloading and isolation. Use functional options to change these settings:

```go
iface, err := vmnet.StartInterfaceWithNetwork(network,
	vmnet.WithInterfaceChecksumOffload(true),
	vmnet.WithInterfaceTSO(true),
	vmnet.WithInterfaceIsolation(true),
	vmnet.WithInterfaceMACAllocation(false),
)
if err != nil {
	return err
}
defer iface.Stop()
```

Options are applied in order. If an option appears more than once, the last value takes precedence.

If you enable TSO, the peer must handle large Ethernet frames. The `Adapter` forwards frames up to the maximum size reported by the SDK. For krunkit offloading, specify `offloading=on` in the device arguments.

## Limits

Datagram send errors with `ENOBUFS` and vmnet send errors with `VMNET_BUFFER_EXHAUSTED` indicate temporary queue congestion. In both cases, the `Adapter` drops the frame and continues forwarding. Other forwarding errors stop the `Adapter` and are returned by `Adapter.Close`.

This package does not support virtio headers or packet batching.

The [vmnet package](../vmnet/README.md) provides network serialization, XPC sharing,
port forwarding, and network configuration options.

Protocol reference material:

* [QEMU socket implementation](https://github.com/qemu/qemu/blob/master/net/socket.c)
* [QEMU stream implementation](https://github.com/qemu/qemu/blob/master/net/stream.c)
* [krunkit device options](https://github.com/libkrun/krunkit/blob/main/docs/usage.md)
* [Lima #4394](https://github.com/lima-vm/lima/pull/4394)
