//go:build darwin

package main

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	guiexample "github.com/Code-Hex/vz/example/go-native-gui"
	"github.com/Code-Hex/vz/v4"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type mygoDemo struct {
	opts                            guiexample.Options
	guest                           *guiexample.Guest
	window                          *mygo.Window
	view                            *vz.VirtualMachineView
	busy, closing, quitting, closed bool
	status                          string
	result                          error
	done                            chan struct{}
	changes                         chan vz.VirtualMachineState
	workers                         sync.WaitGroup
}

type mygoOperation struct {
	demo   *mygoDemo
	name   string
	action func() error
	err    error
}

type mygoLayout struct {
	demo    *mygoDemo
	context *ui.Context
}

func run(opts guiexample.Options, finish func(error)) { finish(runMyGo(opts)) }

func runMyGo(opts guiexample.Options) (result error) {
	guest, err := guiexample.OpenGuest(opts)
	if err != nil {
		return err
	}
	d := &mygoDemo{
		opts:    opts,
		guest:   guest,
		done:    make(chan struct{}),
		changes: make(chan vz.VirtualMachineState, 32),
		status:  "Ready. Stop forces power off.",
	}
	defer d.closeGuest(&result)
	unsubscribe, err := guest.VM.StateChangedNotify(d.changes)
	if err != nil {
		return err
	}
	defer unsubscribe()
	defer d.workers.Wait()
	defer close(d.done)
	if !guest.Bootable {
		d.status = "Display preview. Supply -iso to enable Start."
	}
	mygo.App.WhenReady(d.ready)
	runErr := mygo.App.Run()
	return errors.Join(d.result, runErr)
}

func (d *mygoDemo) closeGuest(result *error) {
	*result = errors.Join(*result, d.guest.Close())
}

func (d *mygoDemo) launch(name string, action func() error) {
	if d.busy || d.closing || d.closed {
		return
	}
	d.busy, d.status = true, name+"..."
	d.workers.Add(1)
	job := &mygoOperation{
		demo:   d,
		name:   name,
		action: action,
	}
	go job.execute()
}

func (job *mygoOperation) execute() {
	defer job.demo.workers.Done()
	job.err = job.action()
	job.demo.window.Update(job.complete)
}

func (job *mygoOperation) complete() {
	d := job.demo
	if d.closed {
		return
	}
	d.busy = false
	d.status = job.name + " completed"
	if job.err != nil {
		d.status = job.name + ": " + job.err.Error()
	}
	if d.quitting {
		mygo.App.Quit()
	} else if d.closing {
		d.window.Close()
	}
}

func (d *mygoDemo) ready() {
	var err error
	d.view, err = vz.NewVirtualMachineView(d.guest.VM)
	if err == nil {
		err = d.view.SetCapturesSystemKeys(d.opts.CaptureKeys)
	}
	if err == nil && d.opts.AutoResize {
		err = d.view.SetAutomaticallyReconfiguresDisplay(true)
	}
	if err != nil {
		d.result = err
		if d.view != nil {
			d.result = errors.Join(d.result, d.view.Close())
		}
		mygo.App.Quit()
		return
	}
	d.window = mygo.NewWindow(mygo.WindowOptions{
		Title:   "MyGo Virtualization",
		Width:   1100,
		Height:  850,
		Content: ui.View(d.content),
	})
	d.window.OnClose(d.onClose)
	offQuit := mygo.App.OnBeforeQuit(d.beforeQuit)
	d.window.OnClosed(offQuit)
	if err := d.view.WithNativeView(d.mount); err != nil {
		d.result = err
		d.window.Close()
		return
	}
	d.workers.Add(1)
	go d.watchState()
	if d.opts.Autostart {
		d.launch("Start", d.startVM)
	}
}

func (d *mygoDemo) content(c *ui.Context) {
	layout := mygoLayout{
		demo:    d,
		context: c,
	}
	ui.Column(c).Padding(12).Gap(8).Children(layout.body)
}

func (l mygoLayout) body() {
	ui.Row(l.context).Gap(8).Children(l.controls)
	ui.Text(l.context, fmt.Sprintf("State: %v | %s", l.demo.guest.VM.State(), l.demo.status))
}

func (l mygoLayout) controls() {
	d, vm := l.demo, l.demo.guest.VM
	for _, command := range []struct {
		name    string
		enabled bool
		run     func() error
	}{
		{
			"Start",
			d.guest.Bootable && vm.CanStart(),
			d.startVM,
		},
		{
			"Pause",
			vm.CanPause(),
			vm.Pause,
		},
		{
			"Resume",
			vm.CanResume(),
			vm.Resume,
		},
		{
			"RequestStop",
			vm.CanRequestStop(),
			d.requestStopVM,
		},
		{
			"Stop",
			vm.CanStop(),
			vm.Stop,
		},
	} {
		if ui.Button(l.context, command.name).Disabled(d.busy || d.closing || d.closed || !command.enabled).Clicked() {
			d.launch(command.name, command.run)
		}
	}
}

func (d *mygoDemo) onClose(event *mygo.CloseEvent) {
	d.closing = true
	if d.busy {
		event.PreventDefault()
		return
	}
	if !d.closed {
		d.closed = true
		d.result = errors.Join(d.result, d.view.Close())
	}
}

func (d *mygoDemo) beforeQuit(event *mygo.QuitEvent) {
	d.quitting, d.closing = true, true
	if d.busy {
		event.PreventDefault()
	}
}

func (d *mygoDemo) mount(pointer unsafe.Pointer) error {
	host := objc.ID(d.window.NativeHandle())
	if host == 0 {
		return errors.New("MyGo returned a nil NSWindow")
	}
	parent := objc.Send[objc.ID](host, objc.RegisterName("contentView"))
	child := objc.ID(uintptr(pointer))
	objc.Send[uintptr](child, objc.RegisterName("setTranslatesAutoresizingMaskIntoConstraints:"), false)
	objc.Send[uintptr](parent, objc.RegisterName("addSubview:"), child)
	for _, edge := range []string{"leadingAnchor", "trailingAnchor", "bottomAnchor"} {
		anchor := objc.Send[objc.ID](child, objc.RegisterName(edge))
		parentAnchor := objc.Send[objc.ID](parent, objc.RegisterName(edge))
		constraint := objc.Send[objc.ID](anchor, objc.RegisterName("constraintEqualToAnchor:"), parentAnchor)
		objc.Send[uintptr](constraint, objc.RegisterName("setActive:"), true)
	}
	top := objc.Send[objc.ID](child, objc.RegisterName("topAnchor"))
	parentTop := objc.Send[objc.ID](parent, objc.RegisterName("topAnchor"))
	constraint := objc.Send[objc.ID](top, objc.RegisterName("constraintEqualToAnchor:constant:"), parentTop, float64(100))
	objc.Send[uintptr](constraint, objc.RegisterName("setActive:"), true)
	objc.Send[uintptr](parent, objc.RegisterName("layoutSubtreeIfNeeded"))
	if objc.Send[objc.ID](child, objc.RegisterName("superview")) != parent {
		return errors.New("MyGo native view mount failed")
	}
	objc.Send[bool](host, objc.RegisterName("makeFirstResponder:"), child)
	return nil
}

func (d *mygoDemo) watchState() {
	defer d.workers.Done()
	for {
		select {
		case <-d.done:
			return
		case <-d.changes:
			d.window.Invalidate()
		}
	}
}

func (d *mygoDemo) startVM() error { return d.guest.VM.Start() }

func (d *mygoDemo) requestStopVM() error {
	_, err := d.guest.VM.RequestStop()
	return err
}

func main() { guiexample.Main(run) }
