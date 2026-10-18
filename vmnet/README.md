# Share a network between processes

These APIs require macOS 26 or later.

`CopySerialization` returns an opaque XPC object. `NewNetworkWithSerialization`
restores a network from that object. The serialization is not a byte encoding
and cannot be written to a file.

Use `ListenNetwork` and `ReceiveNetwork` to share a network through a launchd
Mach service. The server publishes one network:

```go
service, err := vmnet.ListenNetwork("com.example.vmnetwork", network, clientRequirement)
if err != nil {
	return err
}
defer service.Close()
```

The client obtains its own network reference:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
network, err := vmnet.ReceiveNetwork(ctx, "com.example.vmnetwork", serverRequirement)
if err != nil {
	return err
}
```

Pass the received network to `vz.NewVMNetNetworkDeviceAttachment` or
`vmnet.StartInterfaceWithNetwork`. Keep the service open and its process
running while clients use the shared network. The service retains the source
network until it closes. A received reference does not preserve the source
network reservation after the server releases it.

Register the service name in the server's launchd property list:

```xml
<key>MachServices</key>
<dict>
    <key>com.example.vmnetwork</key>
    <true/>
</dict>
```

The property list also needs `Label` and `ProgramArguments`. Load a user agent
with `launchctl bootstrap gui/$(id -u) /path/to/service.plist` and remove it with
`launchctl bootout gui/$(id -u)/com.example.vmnetwork`, using its label in the
second command. `ListenNetwork` does not register the name with launchd.
Connection and registration failures can be reported asynchronously by XPC.
Use a deadline when receiving a network.

Both functions require an Apple code signing requirement expression for the
remote executable. For binaries signed with a Developer ID certificate, a
requirement can combine the signing identifier and team identifier:

```text
anchor apple generic and identifier "com.example.vmhost" and certificate leaf[subject.OU] = "TEAMID1234"
```

For development with one ad hoc signed executable, use `cdhash H"..."` with the
hash printed by `codesign -d -vvvv /path/to/executable`. Rebuilding or signing the
executable changes its hash. The server checks clients and each client checks
the server. An empty requirement is rejected. A requirement of `true` accepts
any executable and should not be used for a service that exposes a private
network.
