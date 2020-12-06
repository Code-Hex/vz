module github.com/Code-Hex/vz/example

go 1.16

replace github.com/Code-Hex/vz => ../

require (
	github.com/Code-Hex/vz v0.0.0-00010101000000-000000000000
	github.com/creack/pty v1.1.11 // indirect
	github.com/kr/pty v1.1.8
	github.com/pkg/term v1.1.0
	golang.org/x/sys v0.0.0-20201207223542-d4d67f95c62d
)
