module github.com/Code-Hex/vz/example/macOS

go 1.22.0

toolchain go1.23.4

replace github.com/Code-Hex/vz/v3 => ../../

require github.com/Code-Hex/vz/v3 v3.0.0-00010101000000-000000000000

require (
	github.com/Code-Hex/go-infinity-channel v1.0.0 // indirect
	golang.org/x/mod v0.22.0 // indirect
)
