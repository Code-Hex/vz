package datagram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"sync"
	"syscall"

	"github.com/Code-Hex/vz/v3/vmnet"
)

// FileAdapterForInterface creates a connected datagram socket for an Interface.
// The caller owns the returned file and must close it. Run forwards packets until
// ctx is canceled, the file is closed, or forwarding fails. Run stops the Interface.
// Invoke Run in a goroutine while the file is in use.
func FileAdapterForInterface(ctx context.Context, iface *vmnet.Interface) (file *os.File, run func() error, err error) {
	if ctx == nil || iface == nil {
		return nil, nil, fmt.Errorf("context and interface are required")
	}
	if iface.MaxPacketSize == 0 || iface.MaxPacketSize >= math.MaxInt || iface.MaxReadPacketCount < 1 || iface.MaxWritePacketCount < 1 {
		return nil, nil, fmt.Errorf("invalid interface packet limits")
	}
	connFile, passingFile, err := socketPair()
	if err != nil {
		return nil, nil, err
	}
	conn, err := net.FileConn(connFile)
	_ = connFile.Close()
	if err != nil {
		_ = passingFile.Close()
		return nil, nil, fmt.Errorf("open datagram connection: %w", err)
	}
	return passingFile, sync.OnceValue(func() error {
		defer conn.Close()
		return forward(ctx, iface, conn)
	}), nil
}

func socketPair() (*os.File, *os.File, error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("create datagram socket pair: %w", err)
	}
	for _, fd := range fds {
		if err := syscall.SetNonblock(fd, true); err != nil {
			_ = syscall.Close(fds[0])
			_ = syscall.Close(fds[1])
			return nil, nil, fmt.Errorf("set datagram socket nonblocking: %w", err)
		}
	}
	return os.NewFile(uintptr(fds[0]), "vmnet-adapter"), os.NewFile(uintptr(fds[1]), "vmnet-device"), nil
}

func forward(ctx context.Context, iface *vmnet.Interface, conn net.Conn) (runErr error) {
	defer func() {
		runErr = errors.Join(runErr, iface.Stop())
	}()
	readManager, err := vmnet.NewPktDescsManager(iface.MaxReadPacketCount, iface.MaxPacketSize)
	if err != nil {
		return err
	}
	writeManager, err := vmnet.NewPktDescsManager(1, iface.MaxPacketSize)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()

	available := make(chan struct{}, 1)
	if err := iface.SetPacketsAvailableEventCallback(func(int) {
		select {
		case available <- struct{}{}:
		default:
		}
	}); err != nil {
		return err
	}
	defer func() {
		runErr = errors.Join(runErr, iface.SetPacketsAvailableEventCallback(nil))
	}()

	outputResult := make(chan error, 1)
	go func() {
		outputResult <- interfaceToConn(ctx, iface, conn, readManager, available)
		cancel()
	}()
	// Read once even if packets arrived before callback registration.
	select {
	case available <- struct{}{}:
	default:
	}

	buffer := make([]byte, int(iface.MaxPacketSize)+1)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if ctx.Err() == nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				runErr = fmt.Errorf("read device packet: %w", err)
			}
			break
		}
		if n > int(iface.MaxPacketSize) {
			runErr = fmt.Errorf("device packet size %d exceeds maximum %d", n, iface.MaxPacketSize)
			break
		}
		if err := writeManager.SetPacket(0, buffer[:n]); err != nil {
			runErr = err
			break
		}
		if err := iface.WritePackets(writeManager, 1); err != nil {
			runErr = fmt.Errorf("write vmnet packet: %w", err)
			break
		}
	}
	cancel()
	_ = conn.Close()
	return errors.Join(runErr, <-outputResult)
}

func interfaceToConn(ctx context.Context, iface *vmnet.Interface, conn net.Conn, manager *vmnet.PktDescsManager, available <-chan struct{}) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-available:
		}
		for {
			if ctx.Err() != nil {
				return nil
			}
			count, err := iface.ReadPackets(manager, iface.MaxReadPacketCount)
			if err != nil {
				return fmt.Errorf("read vmnet packets: %w", err)
			}
			if count == 0 {
				break
			}
			for index := range count {
				packet, err := manager.Packet(index)
				if err != nil {
					return err
				}
				n, err := conn.Write(packet)
				if err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return fmt.Errorf("write device packet: %w", err)
				}
				if n != len(packet) {
					return io.ErrShortWrite
				}
			}
		}
	}
}
