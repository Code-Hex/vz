package xpc_test

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"text/template"

	"github.com/Code-Hex/vz/v3/internal/osversion"
	"github.com/Code-Hex/vz/v3/xpc"
)

var macOSAvailable = osversion.MacOSAvailable

var server bool

func init() {
	// Determine if running as server or client based on command-line arguments
	flag.BoolVar(&server, "server", false, "run as mach service server")
}

func TestMachService(t *testing.T) {
	if err := macOSAvailable(14); err != nil {
		t.Skip("xpc listener is supported from macOS 14")
	}

	label := "dev.code-hex.vz.xpc.test"
	machServiceName := label + ".greeting"

	if server {
		t.Log("running as mach service server")
		listener, err := xpcGreetingServer(t, machServiceName)
		if err != nil {
			log.Printf("failed to create mach service server: %v", err)
			t.Fatal(err)
		}
		if err := listener.Activate(); err != nil {
			log.Printf("failed to activate mach service server: %v", err)
			t.Fatal(err)
		}
		ctx, stop := signal.NotifyContext(t.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		<-ctx.Done()
		_ = listener.Close()
	} else {
		t.Log("running as mach service client")
		xpcRegisterMachService(t, label, machServiceName)
		greeting := "Hello, Mach Service!"
		greetingReply, err := xpcClientRequestingGreeting(t, machServiceName, greeting)
		if err != nil {
			t.Fatal(err)
		}
		if greetingReply != greeting {
			t.Fatalf("expected greeting reply %q to equal greeting %q", greetingReply, greeting)
		}
	}
}

// xpcGreetingServer creates an Mach service XPC listener that echoes back greetings.
func xpcGreetingServer(t *testing.T, machServiceName string) (*xpc.Listener, error) {
	return xpc.NewListener(
		machServiceName,
		xpc.Accept(
			xpc.MessageHandler(func(dic *xpc.Dictionary) *xpc.Dictionary {
				createErrorReply := func(errMsg string, args ...any) *xpc.Dictionary {
					errorString := fmt.Sprintf(errMsg, args...)
					log.Print(errorString)
					t.Error(errorString)
					return dic.CreateReply(
						xpc.KeyValue("Error", xpc.NewString(errorString)),
					)
				}
				var reply *xpc.Dictionary
				if greeting := dic.GetString("Greeting"); greeting == "" {
					reply = createErrorReply("missing greeting in request")
				} else {
					reply = dic.CreateReply(
						xpc.KeyValue("Greeting", xpc.NewString(greeting)),
					)
				}
				return reply
			}),
		),
	)
}

const launchdPlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
    <dict>
        <key>Label</key>
        <string>{{.Label}}</string>
        <key>ProgramArguments</key>
        <array>
            {{- range $arg := .ProgramArguments}}
            <string>{{$arg}}</string>
            {{- end}}
        </array>
        <key>RunAtLoad</key>
        <true/>
        <key>WorkingDirectory</key>
        <string>{{ .WorkingDirectory }}</string>
        <key>StandardErrorPath</key>
        <string>{{ .WorkingDirectory }}/xpc_test.stderr.log</string>
        <!-- <key>StandardOutPath</key>
        <string>{{ .WorkingDirectory }}/xpc_test.stdout.log</string> -->
        <key>MachServices</key>
        <dict>
            {{- range $service := .MachServices}}
            <key>{{$service}}</key>
            <true/>
            {{- end}}
        </dict>
    </dict>
</plist>`

// xpcRegisterMachService registers the test executable as a Mach service
// using launchctl with the given label and machServiceName.
func xpcRegisterMachService(t *testing.T, label, machServiceName string) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	params := struct {
		Label            string
		ProgramArguments []string
		WorkingDirectory string
		MachServices     []string
	}{
		Label:            label,
		ProgramArguments: []string{os.Args[0], "-test.run", "^" + funcName(t, 2) + "$", "-server"},
		WorkingDirectory: cwd,
		MachServices:     []string{machServiceName},
	}
	template, err := template.New("plist").Parse(launchdPlistTemplate)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := template.Execute(&b, params); err != nil {
		t.Fatal(err)
	}
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	launchAgentDir := path.Join(userHomeDir, "Library", "LaunchAgents", label+".plist")
	if err := os.WriteFile(launchAgentDir, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Remove(launchAgentDir); err != nil {
			t.Logf("failed to remove launch agent plist: %v", err)
		}
	})
	cmd := exec.CommandContext(t.Context(), "launchctl", "load", launchAgentDir)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// do not use t.Context() here to ensure unload runs
		cmd := exec.CommandContext(context.Background(), "launchctl", "unload", launchAgentDir)
		if err := cmd.Run(); err != nil {
			t.Logf("failed to unload launch agent: %v", err)
		}
	})
}

// funcName returns the name of the calling function.
// It is used to get the test function name for launchctl registration.
func funcName(t *testing.T, skip int) string {
	pc, _, _, ok := runtime.Caller(skip)
	if !ok {
		t.Fatal("failed to get caller info")
	}
	funcNameComponents := strings.Split(runtime.FuncForPC(pc).Name(), ".")
	return funcNameComponents[len(funcNameComponents)-1]
}

// xpcClientRequestingGreeting requests a VmnetNetwork serialization for the given subnet
// from the Mach service and returns the deserialized VmnetNetwork instance.
func xpcClientRequestingGreeting(t *testing.T, machServiceName, greeting string) (string, error) {
	session, err := xpc.NewSession(
		machServiceName,
	)
	if err != nil {
		return "", err
	}
	defer session.Cancel()

	resp, err := session.SendDictionaryWithReply(
		t.Context(), xpc.KeyValue("Greeting", xpc.NewString(greeting)),
	)
	if err != nil {
		return "", err
	}
	errorStr := resp.GetString("Error")
	if errorStr != "" {
		return "", fmt.Errorf("xpc service error: %s", errorStr)
	}
	greetingReply := resp.GetString("Greeting")
	return greetingReply, nil
}
