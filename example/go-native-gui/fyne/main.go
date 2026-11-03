//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit
#include "native.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/widget"
	guiexample "github.com/Code-Hex/vz/example/go-native-gui"
	vz "github.com/Code-Hex/vz/v4"
)

type fyneDemo struct {
	opts         guiexample.Options
	guest        *guiexample.Guest
	window       fyne.Window
	status       *widget.Label
	view         *vz.VirtualMachineView
	busy, closed bool
	done         chan struct{}
	workers      sync.WaitGroup
	result       error
	nativeErr    error
}

type fyneOperation struct {
	demo   *fyneDemo
	name   string
	action func() error
	err    error
}

type fyneMount struct {
	host uintptr
}

func run(opts guiexample.Options, finish func(error)) { finish(runFyne(opts)) }

func runFyne(opts guiexample.Options) (result error) {
	guest, err := guiexample.OpenGuest(opts)
	if err != nil {
		return err
	}
	d := &fyneDemo{opts: opts, guest: guest, done: make(chan struct{})}
	defer d.closeGuest(&result)
	application := app.NewWithID("dev.codehex.vz.go-native-gui.fyne")
	d.window = application.NewWindow("Fyne Virtualization")
	d.status = widget.NewLabel("Ready. Stop forces power off.")
	if !guest.Bootable {
		d.status.SetText("Display preview. Supply -iso to start.")
	}
	d.window.SetContent(container.NewBorder(container.NewVBox(d.controls(), d.status), nil, nil, nil))
	d.window.Resize(fyne.NewSize(1024, 868))
	d.window.SetOnClosed(d.onClosed)
	application.Lifecycle().SetOnStarted(d.onStarted)
	d.window.ShowAndRun()
	d.workers.Wait()
	return d.result
}

func (d *fyneDemo) closeGuest(result *error) {
	*result = errors.Join(*result, d.guest.Close())
}

func (d *fyneDemo) controls() *fyne.Container {
	start := widget.NewButton("Start", d.start)
	if !d.guest.Bootable {
		start.Disable()
	}
	return container.NewHBox(start,
		widget.NewButton("Pause", d.pause),
		widget.NewButton("Resume", d.resume),
		widget.NewButton("RequestStop", d.requestStop),
		widget.NewButton("Stop", d.stop))
}

func (d *fyneDemo) start() {
	d.action("Start", d.startVM)
}

func (d *fyneDemo) pause() {
	d.action("Pause", d.guest.VM.Pause)
}

func (d *fyneDemo) resume() {
	d.action("Resume", d.guest.VM.Resume)
}

func (d *fyneDemo) requestStop() {
	d.action("RequestStop", d.requestStopVM)
}

func (d *fyneDemo) stop() {
	d.action("Stop", d.guest.VM.Stop)
}

func (d *fyneDemo) startVM() error {
	return d.guest.VM.Start()
}

func (d *fyneDemo) requestStopVM() error {
	_, err := d.guest.VM.RequestStop()
	return err
}

func (d *fyneDemo) action(name string, operation func() error) {
	if d.closed || d.busy || d.view == nil {
		return
	}
	d.busy = true
	d.status.SetText(name + "...")
	job := &fyneOperation{demo: d, name: name, action: operation}
	d.workers.Add(1)
	go job.execute()
}

func (job *fyneOperation) execute() {
	defer job.demo.workers.Done()
	job.err = job.action()
	select {
	case <-job.demo.done:
		return
	default:
	}
	fyne.Do(job.complete)
}

func (job *fyneOperation) complete() {
	d := job.demo
	if d.closed {
		return
	}
	d.busy = false
	if job.err != nil {
		d.status.SetText(job.name + ": " + job.err.Error())
	} else {
		d.status.SetText(fmt.Sprintf("%s: %v", job.name, d.guest.VM.State()))
	}
}

func (d *fyneDemo) onClosed() {
	d.closed = true
	close(d.done)
	if d.view != nil {
		d.window.(driver.NativeWindow).RunNative(d.closeNative)
	}
}

func (d *fyneDemo) closeNative(any) {
	d.result = errors.Join(d.result, d.view.Close())
}

func (d *fyneDemo) onStarted() {
	fyne.Do(d.mountOnUI)
}

func (d *fyneDemo) mountOnUI() {
	if d.closed {
		return
	}
	d.nativeErr = nil
	d.window.(driver.NativeWindow).RunNative(d.mountNative)
	if d.nativeErr != nil {
		d.result = errors.Join(d.result, d.nativeErr)
		d.window.Close()
		return
	}
	if d.opts.Autostart {
		d.start()
	}
}

func (d *fyneDemo) mountNative(context any) {
	d.view, d.nativeErr = vz.NewVirtualMachineView(d.guest.VM)
	if d.nativeErr != nil {
		return
	}
	if d.nativeErr = d.view.SetCapturesSystemKeys(d.opts.CaptureKeys); d.nativeErr != nil {
		return
	}
	if d.opts.AutoResize {
		if d.nativeErr = d.view.SetAutomaticallyReconfiguresDisplay(true); d.nativeErr != nil {
			return
		}
	}
	mount := fyneMount{host: context.(driver.MacWindowContext).NSWindow}
	d.nativeErr = d.view.WithNativeView(mount.attach)
}

func (mount fyneMount) attach(pointer unsafe.Pointer) error {
	if C.vzGUIFyneMount(C.uintptr_t(mount.host), pointer) == 0 {
		return fmt.Errorf("Fyne native view mount failed")
	}
	return nil
}

func main() { guiexample.Main(run) }
