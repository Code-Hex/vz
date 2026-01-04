module github.com/Code-Hex/vz/example/linux

go 1.24.0

replace github.com/Code-Hex/vz/v3 => ../../

require (
	github.com/Code-Hex/vz/v3 v3.0.0-00010101000000-000000000000
	github.com/pkg/term v1.1.0
	golang.org/x/sys v0.39.0
)

require (
	github.com/Code-Hex/go-infinity-channel v1.0.0 // indirect
	golang.org/x/mod v0.22.0 // indirect
)
