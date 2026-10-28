package vmnet_test

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/Code-Hex/vz/v4/vmnet"
)

func ExampleInterface_ReadPackets() {
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		fmt.Println(err)
		return
	}
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		fmt.Println(err)
		return
	}
	iface, err := vmnet.StartInterfaceWithNetwork(network)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer iface.Stop()

	frames := make([][]byte, 32)
	for j := range frames {
		frames[j] = make([]byte, 0, iface.MaxPacketSize())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		n, err := iface.ReadPackets(ctx, frames)
		if err != nil {
			fmt.Println(err)
			return
		}
		for _, frame := range frames[:n] {
			fmt.Printf("received %d bytes\n", len(frame))
		}
		// Copy any frames you need to keep before the next read reuses the buffers.
	}
}

func ExampleInterface_WritePackets() {
	config, err := vmnet.NewNetworkConfiguration(vmnet.ModeHost)
	if err != nil {
		fmt.Println(err)
		return
	}
	network, err := vmnet.NewNetwork(config)
	if err != nil {
		fmt.Println(err)
		return
	}
	iface, err := vmnet.StartInterfaceWithNetwork(network)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer iface.Stop()

	frames := make([][]byte, 2)
	for j := range frames {
		frame := make([]byte, 60)
		copy(frame[:6], []byte{255, 255, 255, 255, 255, 255})
		copy(frame[6:12], iface.MACAddress())
		// EtherType 0x88b5 is reserved for local experimental use.
		binary.BigEndian.PutUint16(frame[12:14], 0x88b5)
		copy(frame[14:], fmt.Sprintf("packet %d", j))
		frames[j] = frame
	}
	if n, err := iface.WritePackets(frames); err != nil {
		// The failed native call may have sent more than n frames. Do not retry
		// frames[n:] unless the protocol can handle duplicates.
		fmt.Printf("sent at least %d frames: %v\n", n, err)
		return
	}
}
