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
	"runtime/cgo"
	"sync"
	"unsafe"

	"gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	guiexample "github.com/Code-Hex/vz/example/go-native-gui"
	vz "github.com/Code-Hex/vz/v4"
)

type operationResult struct {
	name string
	err  error
}

type gioDemo struct {
	opts                       guiexample.Options
	finish                     func(error)
	guest                      *guiexample.Guest
	window                     *app.Window
	view                       *vz.VirtualMachineView
	host                       uintptr
	closed, observingClose     bool
	busy, closing, started     bool
	result, nativeErr          error
	status                     string
	changes                    chan vz.VirtualMachineState
	done                       chan struct{}
	completions                chan operationResult
	nativeClosed, quitRequests chan struct{}
	eventRequests              chan struct{}
	events                     chan event.Event
	workers                    sync.WaitGroup
	buttons                    [5]widget.Clickable
	focus                      widget.Clickable
	ops                        op.Ops
	theme                      *material.Theme
}

type gioOperation struct {
	demo   *gioDemo
	name   string
	action func() error
}

type gioButton struct {
	demo      *gioDemo
	clickable *widget.Clickable
	label     string
	enabled   bool
}

//export vzGUIGioWindowWillClose
func vzGUIGioWindowWillClose(value C.uintptr_t) {
	handle := cgo.Handle(value)
	defer handle.Delete()
	handle.Value().(*gioDemo).notifyClosed()
}

//export vzGUIGioApplicationShouldTerminate
func vzGUIGioApplicationShouldTerminate(value C.uintptr_t) {
	cgo.Handle(value).Value().(*gioDemo).notifyQuit()
}

func run(opts guiexample.Options, finish func(error)) {
	d := &gioDemo{
		opts:   opts,
		finish: finish,
		window: new(app.Window),
	}
	go d.run()
	app.Main()
}

func (d *gioDemo) run() {
	d.finish(d.runWindow())
}

func (d *gioDemo) runWindow() (result error) {
	guest, err := guiexample.OpenGuest(d.opts)
	if err != nil {
		return err
	}
	d.guest = guest
	defer d.closeGuest(&result)
	defer d.workers.Wait()
	d.changes = make(chan vz.VirtualMachineState, 32)
	unsubscribe, err := guest.VM.StateChangedNotify(d.changes)
	if err != nil {
		return err
	}
	defer unsubscribe()

	d.status = "Ready"
	if !guest.Bootable {
		d.status = "Unstarted VM preview, supply -iso"
	}
	d.theme = material.NewTheme()
	d.completions = make(chan operationResult, 1)
	d.done = make(chan struct{})
	d.nativeClosed = make(chan struct{}, 1)
	d.quitRequests = make(chan struct{}, 1)
	d.eventRequests = make(chan struct{}, 1)
	d.events = make(chan event.Event)
	defer close(d.done)
	go d.watchState()
	d.window.Option(app.Title("Gio native VM overlay"), app.Size(unit.Dp(800), unit.Dp(660)))
	go d.pumpEvents()
	return d.eventLoop()
}

func (d *gioDemo) closeGuest(result *error) {
	*result = errors.Join(*result, d.guest.Close())
}

func (d *gioDemo) watchState() {
	for {
		select {
		case <-d.done:
			return
		case <-d.changes:
			d.window.Invalidate()
		}
	}
}

func (d *gioDemo) launch(name string, action func() error) {
	if d.busy {
		return
	}
	d.busy = true
	d.status = name + "..."
	d.workers.Add(1)
	job := &gioOperation{
		demo:   d,
		name:   name,
		action: action,
	}
	go job.execute()
}

func (job *gioOperation) execute() {
	defer job.demo.workers.Done()
	job.demo.completions <- operationResult{
		job.name,
		job.action(),
	}
}

func (d *gioDemo) closeView() {
	if d.view == nil {
		d.closed = true
		return
	}
	if d.closed || d.host == 0 {
		return
	}
	d.nativeErr = nil
	d.window.Run(d.closeNativeView)
	if d.nativeErr != nil {
		d.result = d.nativeErr
	} else {
		d.closed = true
	}
}

func (d *gioDemo) closeNativeView() {
	C.vzGUIGioRestoreHostFocus(C.uintptr_t(d.host))
	d.nativeErr = d.view.Close()
}

func (d *gioDemo) finishClose() {
	d.closeView()
	d.window.Run(d.scheduleHostClose)
}

func (d *gioDemo) scheduleHostClose() {
	C.vzGUIGioScheduleHostClose(C.uintptr_t(d.host))
}

func (d *gioDemo) requestClose() {
	d.closing = true
	if d.busy {
		return
	}
	if d.guest.VM.CanStop() {
		d.launch("Stop", d.guest.VM.Stop)
		return
	}
	d.finishClose()
}

func (d *gioDemo) completedClose() error {
	if d.result != nil {
		return d.result
	}
	if !d.closed {
		return fmt.Errorf("window closed before native cleanup")
	}
	return nil
}

func (d *gioDemo) handleCompletion(completed operationResult) {
	d.busy = false
	d.status = completed.name + " completed"
	if completed.err != nil {
		d.status = completed.name + ": " + completed.err.Error()
		if completed.name == "Start" && d.opts.Autostart {
			d.result = completed.err
			d.closing = true
		}
		if d.closing && completed.name == "Stop" {
			d.result = completed.err
			d.finishClose()
		}
	}
	if d.closing && !(completed.name == "Stop" && completed.err != nil) {
		d.requestClose()
	}
	d.window.Invalidate()
}

func (d *gioDemo) handleView(e app.AppKitViewEvent) {
	if !e.Valid() {
		if d.view != nil && !d.closed {
			d.result = fmt.Errorf("native handle invalidated before view cleanup")
		}
		d.host = 0
		return
	}
	if d.view != nil && !d.closed && d.host == e.View {
		return
	}
	d.closeView()
	d.host = e.View
	d.closed = false
	d.nativeErr = nil
	d.window.Run(d.mountView)
	if d.nativeErr != nil {
		d.result = d.nativeErr
		d.finishClose()
		return
	}
	if d.opts.Autostart && !d.started {
		d.started = true
		d.launch("Start", d.startVM)
	}
}

func (d *gioDemo) mountView() {
	d.view, d.nativeErr = vz.NewVirtualMachineView(d.guest.VM)
	if d.nativeErr != nil {
		return
	}
	if !d.observingClose {
		handle := cgo.NewHandle(d)
		if C.vzGUIGioObserveWindowClose(C.uintptr_t(d.host), C.uintptr_t(handle)) == 0 {
			handle.Delete()
			d.nativeErr = fmt.Errorf("native close observer requires NSWindow")
			return
		}
		d.observingClose = true
	}
	d.nativeErr = d.view.WithNativeView(d.attachView)
	if d.nativeErr == nil {
		d.nativeErr = d.view.SetCapturesSystemKeys(d.opts.CaptureKeys)
	}
	if d.nativeErr == nil && d.opts.AutoResize {
		d.nativeErr = d.view.SetAutomaticallyReconfiguresDisplay(true)
	}
}

func (d *gioDemo) attachView(pointer unsafe.Pointer) error {
	if C.vzGUIGioMountView(C.uintptr_t(d.host), pointer) == 0 {
		return fmt.Errorf("Gio native view mount failed")
	}
	return nil
}

func (d *gioDemo) notifyClosed() {
	select {
	case d.nativeClosed <- struct{}{}:
	default:
	}
}

func (d *gioDemo) notifyQuit() {
	select {
	case d.quitRequests <- struct{}{}:
	default:
	}
}

func (d *gioDemo) pumpEvents() {
	// Calling Event again acknowledges the previous event, so do not read ahead.
	for {
		select {
		case <-d.done:
			return
		case <-d.eventRequests:
		}
		next := d.window.Event()
		select {
		case <-d.done:
			return
		case d.events <- next:
		}
	}
}

func (d *gioDemo) eventLoop() error {
	requestNext := true
	for {
		if requestNext {
			d.eventRequests <- struct{}{}
			requestNext = false
		}
		var current event.Event
		select {
		case current = <-d.events:
			requestNext = true
		case completed := <-d.completions:
			d.handleCompletion(completed)
			continue
		case <-d.quitRequests:
			d.requestClose()
			continue
		case <-d.nativeClosed:
			return d.nativeCloseResult()
		}
		switch e := current.(type) {
		case app.AppKitViewEvent:
			d.handleView(e)
		case *app.ClosingEvent:
			d.handleClosing(e)
		case app.FrameEvent:
			d.frame(e)
		case app.DestroyEvent:
			if e.Err != nil {
				return e.Err
			}
			return d.completedClose()
		}
	}
}

func (d *gioDemo) nativeCloseResult() error {
	select {
	case last := <-d.events:
		if destroyed, ok := last.(app.DestroyEvent); ok && destroyed.Err != nil {
			return destroyed.Err
		}
	default:
	}
	return d.completedClose()
}

func (d *gioDemo) handleClosing(e *app.ClosingEvent) {
	if d.closed {
		return
	}
	if d.busy || d.guest.VM.CanStop() {
		e.Abort()
		d.requestClose()
	} else {
		d.closeView()
	}
}

func (d *gioDemo) frame(e app.FrameEvent) {
	gtx := app.NewContext(&d.ops, e)
	gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(60))
	layout.Inset{
		Left: unit.Dp(8),
		Top:  unit.Dp(3),
	}.Layout(gtx, d.layoutHeader)
	e.Frame(&d.ops)
}

func (d *gioDemo) layoutHeader(gtx layout.Context) layout.Dimensions {
	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		layout.Rigid(d.layoutControls),
		layout.Rigid(d.layoutStatus))
}

func (d *gioDemo) layoutControls(gtx layout.Context) layout.Dimensions {
	vm := d.guest.VM
	labels := []string{"Start", "Pause", "Resume", "Stop", "RequestStop"}
	actions := []func() error{d.startVM, vm.Pause, vm.Resume, vm.Stop, d.requestStopVM}
	enabled := []bool{d.guest.Bootable && vm.CanStart(), vm.CanPause(), vm.CanResume(), vm.CanStop(), vm.CanRequestStop()}
	children := make([]layout.FlexChild, 0, 7)
	for i, label := range labels {
		if d.buttons[i].Clicked(gtx) && !d.busy && !d.closing && enabled[i] {
			d.launch(label, actions[i])
		}
		button := gioButton{
			demo:      d,
			clickable: &d.buttons[i],
			label:     label,
			enabled:   enabled[i],
		}
		children = append(children, layout.Rigid(button.layout))
	}
	children = append(children, layout.Rigid(d.layoutFocus))
	return layout.Flex{}.Layout(gtx, children...)
}

func (b gioButton) layout(gtx layout.Context) layout.Dimensions {
	if b.demo.busy || b.demo.closing || !b.enabled {
		gtx = gtx.Disabled()
	}
	button := material.Button(b.demo.theme, b.clickable, b.label)
	button.TextSize = unit.Sp(12)
	button.Inset = layout.UniformInset(unit.Dp(4))
	return button.Layout(gtx)
}

func (d *gioDemo) layoutFocus(gtx layout.Context) layout.Dimensions {
	if d.focus.Clicked(gtx) && !d.closed && d.view != nil && d.host != 0 {
		d.nativeErr = nil
		d.window.Run(d.focusNativeView)
		if d.nativeErr != nil {
			d.status = "Focus VM: " + d.nativeErr.Error()
		}
	}
	button := material.Button(d.theme, &d.focus, "Focus VM")
	button.TextSize = unit.Sp(12)
	button.Inset = layout.UniformInset(unit.Dp(4))
	return button.Layout(gtx)
}

func (d *gioDemo) focusNativeView() {
	d.nativeErr = d.view.WithNativeView(d.focusView)
}

func (d *gioDemo) focusView(pointer unsafe.Pointer) error {
	C.vzGUIGioFocusView(C.uintptr_t(d.host), pointer)
	return nil
}

func (d *gioDemo) layoutStatus(gtx layout.Context) layout.Dimensions {
	return material.Body2(d.theme, fmt.Sprintf("%v | %s", d.guest.VM.State(), d.status)).Layout(gtx)
}

func (d *gioDemo) startVM() error { return d.guest.VM.Start() }

func (d *gioDemo) requestStopVM() error {
	_, err := d.guest.VM.RequestStop()
	return err
}

func main() { guiexample.Main(run) }
