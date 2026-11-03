//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#include "native.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"unsafe"

	guiexample "github.com/Code-Hex/vz/example/go-native-gui"
	vz "github.com/Code-Hex/vz/v4"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type wailsDemo struct {
	opts                            guiexample.Options
	finish                          func(error)
	guest                           *guiexample.Guest
	app                             *application.App
	window                          *application.WebviewWindow
	view                            *vz.VirtualMachineView
	mu                              sync.Mutex
	ready, busy, closing, allowQuit bool
	status                          string
	err                             error
	workers                         sync.WaitGroup
	mounted                         sync.Once
	cleanup                         sync.Once
	finished                        sync.Once
}

func (d *wailsDemo) fail(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.err = errors.Join(d.err, err)
}

func (d *wailsDemo) closeGuest() {
	d.cleanup.Do(d.releaseGuest)
}

func (d *wailsDemo) releaseGuest() {
	d.workers.Wait()
	d.fail(d.guest.Close())
}

func (d *wailsDemo) closeView() bool {
	d.mu.Lock()
	if d.closing {
		d.mu.Unlock()
		return false
	}
	d.closing = true
	d.mu.Unlock()
	if d.view != nil {
		d.fail(d.view.Close())
		d.view = nil
	}
	return true
}

func (d *wailsDemo) requestClose() {
	application.InvokeAsync(d.beginClose)
}

func (d *wailsDemo) beginClose() {
	if !d.closeView() {
		return
	}
	go d.stopAndQuit()
}

func (d *wailsDemo) stopAndQuit() {
	d.closeGuest()
	d.mu.Lock()
	d.allowQuit = true
	d.mu.Unlock()
	application.InvokeAsync(d.app.Quit)
}

func (d *wailsDemo) action(name string) error {
	d.mu.Lock()
	if d.closing || !d.ready || d.busy {
		d.status = "Window is not ready, busy, or closing."
		d.mu.Unlock()
		return fmt.Errorf("window is not ready, busy, or closing")
	}
	if name == "Start" && !d.guest.Bootable {
		d.status = "Display preview. Supply -iso to start."
		d.mu.Unlock()
		return fmt.Errorf("preview: supply -iso to start")
	}
	d.busy = true
	d.workers.Add(1)
	d.mu.Unlock()
	defer d.workers.Done()
	var err error
	switch name {
	case "Start":
		err = d.guest.VM.Start()
	case "Pause":
		err = d.guest.VM.Pause()
	case "Resume":
		err = d.guest.VM.Resume()
	case "RequestStop":
		_, err = d.guest.VM.RequestStop()
	case "Stop":
		err = d.guest.VM.Stop()
	default:
		err = fmt.Errorf("unknown operation %q", name)
	}
	d.mu.Lock()
	d.busy = false
	if err != nil {
		d.status = name + ": " + err.Error()
	} else {
		d.status = fmt.Sprintf("%s: %v", name, d.guest.VM.State())
	}
	d.mu.Unlock()
	return err
}

func (d *wailsDemo) serve(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/action":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := d.action(r.URL.Query().Get("op")); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		fallthrough
	case "/state":
		d.mu.Lock()
		status := d.status
		d.mu.Unlock()
		fmt.Fprint(w, status)
	default:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		disabled := ""
		if !d.guest.Bootable {
			disabled = " disabled"
		}
		fmt.Fprintf(w, `<!doctype html>
<html>
<head>
<script type="module" src="/wails/runtime.js"></script>
<style>
body { font: 14px system-ui; margin: 12px; background: #eceff3; }
button { padding: 8px; margin-right: 6px; }
#status { margin-top: 8px; white-space: pre-wrap; }
</style>
</head>
<body>
<button%s>Start</button><button>Pause</button><button>Resume</button><button>RequestStop</button><button>Stop</button>
<div id="status"></div>
<script>
const status = document.querySelector('#status');
async function handleAction(event) {
	try {
		const response = await fetch('/action?op=' + event.currentTarget.textContent, {method: 'POST'});
		status.textContent = await response.text();
	} catch (error) {
		status.textContent = String(error);
	}
}
async function refresh() {
	try {
		const response = await fetch('/state');
		status.textContent = await response.text();
	} catch {}
}
for (const button of document.querySelectorAll('button')) {
	button.onclick = handleAction;
}
refresh();
setInterval(refresh, 1000);
</script>
</body>
</html>`, disabled)
	}
}

func (d *wailsDemo) complete() {
	d.finished.Do(d.finishRun)
}

func (d *wailsDemo) finishRun() {
	d.closeGuest()
	d.mu.Lock()
	err := d.err
	d.mu.Unlock()
	d.finish(err)
}

func (d *wailsDemo) shouldQuit() bool {
	d.mu.Lock()
	allowed := d.allowQuit
	d.mu.Unlock()
	if !allowed {
		d.requestClose()
	}
	return allowed
}

func (d *wailsDemo) windowClosing(event *application.WindowEvent) {
	d.mu.Lock()
	allowed := d.allowQuit
	d.mu.Unlock()
	if !allowed {
		event.Cancel()
		d.requestClose()
	}
}

func (d *wailsDemo) shutdown() {
	d.closeView()
}

func (d *wailsDemo) runtimeReady(*application.WindowEvent) {
	d.mounted.Do(d.scheduleMount)
}

func (d *wailsDemo) scheduleMount() {
	application.InvokeAsync(d.mountView)
}

func (d *wailsDemo) mountView() {
	d.mu.Lock()
	closing := d.closing
	d.mu.Unlock()
	if closing {
		return
	}
	var err error
	d.view, err = vz.NewVirtualMachineView(d.guest.VM)
	if err == nil {
		err = d.view.SetCapturesSystemKeys(d.opts.CaptureKeys)
	}
	if err == nil && d.opts.AutoResize {
		err = d.view.SetAutomaticallyReconfiguresDisplay(true)
	}
	if err == nil {
		err = d.view.WithNativeView(d.mountNative)
	}
	if err != nil {
		d.fail(err)
		d.requestClose()
		return
	}
	d.mu.Lock()
	d.ready = true
	d.mu.Unlock()
	if d.opts.Autostart {
		go d.autostart()
	}
}

func (d *wailsDemo) mountNative(pointer unsafe.Pointer) error {
	if !bool(C.vzGUIWailsMount(d.window.NativeWindow(), pointer)) {
		return fmt.Errorf("Wails native view mount failed")
	}
	return nil
}

func (d *wailsDemo) autostart() {
	_ = d.action("Start")
}

func run(opts guiexample.Options, finish func(error)) {
	guest, err := guiexample.OpenGuest(opts)
	if err != nil {
		finish(err)
		return
	}
	d := &wailsDemo{
		opts:   opts,
		finish: finish,
		guest:  guest,
		status: "Ready. Stop forces power off.",
	}
	if !guest.Bootable {
		d.status = "Display preview. Supply -iso to start."
	}
	d.app = application.New(application.Options{
		Name: "VZGoNativeWails",
		Assets: application.AssetOptions{
			Handler: http.HandlerFunc(d.serve),
		},
		ShouldQuit: d.shouldQuit,
		// AppKit termination may never return from Run.
		PostShutdown: d.complete,
	})
	d.window = d.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Wails Virtualization",
		Width:  1024,
		Height: 878,
		URL:    "/",
	})
	d.window.RegisterHook(events.Common.WindowClosing, d.windowClosing)
	d.app.OnShutdown(d.shutdown)
	d.window.OnWindowEvent(events.Common.WindowRuntimeReady, d.runtimeReady)
	d.fail(d.app.Run())
	d.closeView()
	d.complete()
}

func main() { guiexample.Main(run) }
