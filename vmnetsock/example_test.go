package vmnetsock_test

import (
	"context"
	"fmt"

	"github.com/Code-Hex/vz/v4/vmnet"
	"github.com/Code-Hex/vz/v4/vmnetsock"
)

func ExampleNew_peerVirtioHeaderNormalization() {
	configuration, err := vmnet.NewNetworkConfiguration(vmnet.ModeShared)
	if err != nil {
		fmt.Println(err)
		return
	}
	network, err := vmnet.NewNetwork(configuration)
	if err != nil {
		fmt.Println(err)
		return
	}
	iface, err := vmnet.StartInterfaceWithNetwork(network,
		vmnet.WithInterfaceVirtioHeader(true),
		vmnet.WithInterfaceTSO(true),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer iface.Stop()

	adapter, err := vmnetsock.New(context.Background(), iface, vmnetsock.Stream,
		vmnetsock.WithPeerVirtioHeaderNormalization(),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer adapter.Close()
	peer, err := adapter.Dup()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer peer.Close()

	// Pass this caller-owned socket to a peer that exchanges length-prefixed
	// Virtio packets and handles checksum, GSO, and RSC metadata from vmnet.
	fmt.Printf("Virtio peer socket descriptor: %d\n", peer.Fd())
}
