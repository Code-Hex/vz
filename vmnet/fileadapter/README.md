# Connect vmnet to a file descriptor

`vmnet.Interface` exchanges Ethernet frames on an existing `vmnet.Network`. It requires macOS 26 or newer and a compatible SDK. `fileadapter.New` creates a UNIX socket pair and starts forwarding packets in both directions.

`fileadapter.New` supports two socket protocols:

* `fileadapter.Stream` uses QEMU's stream and socket protocol, a four-byte big-endian length followed by an Ethernet frame.
* `fileadapter.Datagram` uses krunkit's unixgram protocol, with one Ethernet frame per datagram.

## Usage example

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
adapter, file, err := fileadapter.New(ctx, iface, fileadapter.Stream)
if err != nil {
	return err
}
defer adapter.Close()
defer file.Close()
cmd := exec.CommandContext(ctx, "qemu-system-aarch64",
	"-netdev", "stream,id=net0,addr.type=fd,addr.str=3",
	"-device", "virtio-net-pci,netdev=net0,mac="+iface.MACAddress().String())
cmd.ExtraFiles = []*os.File{file}
if err := cmd.Start(); err != nil {
	return err
}
if err := file.Close(); err != nil {
	return err
}
if err := cmd.Wait(); err != nil {
	return err
}
return adapter.Close()
```

## Pass the descriptor to the VM

Add the network and device flags to your existing virtual machine configuration. The first file passed to `exec.Cmd.ExtraFiles` maps to file descriptor 3 in the child process.

Stream also supports QEMU's older `-netdev socket,id=net0,fd=3` syntax. For krunkit, select `fileadapter.Datagram` and pass the descriptor and MAC address through `--device virtio-net,type=unixgram,fd=3,mac=...`.

The caller owns the `*os.File` returned by `fileadapter.New`. Keep this file open in the parent process until `cmd.Start` succeeds, then close the parent copy. The child process retains its own duplicated descriptor.

The `Adapter` closes its own socket when forwarding stops. It never stops the borrowed `Interface`.

## Stop forwarding

`Adapter.Close` stops packet forwarding and waits for both forwarding goroutines to exit. `Adapter.Wait` returns the first cause of termination:

* A peer EOF in Stream mode or an explicit call to `Close` returns `nil` if it triggers termination first.
* If a forwarding failure or context cancellation occurs first, subsequent calls to `Close` and `Wait` preserve that original error.
* Context cancellation returns `context.Canceled` or `context.DeadlineExceeded`.

For Datagram, closing the peer descriptor does not reliably interrupt an idle read. Cancel the context or call `Adapter.Close` when the VM exits.

Multiple readers on one `Interface` divide the received packets, so normally attach one `Adapter` per `Interface`.

## Interface lifetime and options

All copies of an `Interface` share a single lifetime. Calling `Stop` invalidates the interface; subsequent reads and writes return `net.ErrClosed`.

The SDK reference from an `Interface` to its `Network` is independent of your application's `Network` reference. Calling `Stop` on an `Interface` does not terminate the underlying `Network`, and you can start a new `Interface` on that same `Network` afterward.

On a startup error, `StartInterfaceWithNetwork` returns a nil `Interface` and handles cleanup internally. If native rollback fails, the library retains the session and retries `Stop` in the background until it succeeds.

With no options, `StartInterfaceWithNetwork` enables MAC allocation and disables offloading and isolation. Use functional options to change these settings:

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

## Limits and verification

Datagram send errors with `ENOBUFS` and vmnet send errors with `VMNET_BUFFER_EXHAUSTED` indicate temporary queue congestion. The `Adapter` drops that frame and keeps the connection open. Other forwarding errors stop the `Adapter` and are returned by `Adapter.Wait`.

This package does not support virtio headers or packet batching. Serialization, XPC sharing, port forwarding, and additional network configuration options remain separate work.

Run `make test/vmnet` to verify frame round trips through the real SDK and the socket protocols. The target signs the test binary with the repository's `cmd/codesign` tool. Communication with running QEMU or krunkit VMs has not been verified.

Protocol reference material:

* [QEMU socket implementation](https://github.com/qemu/qemu/blob/master/net/socket.c)
* [QEMU stream implementation](https://github.com/qemu/qemu/blob/master/net/stream.c)
* [krunkit device options](https://github.com/libkrun/krunkit/blob/main/docs/usage.md)
* [Lima #4394](https://github.com/lima-vm/lima/pull/4394)
