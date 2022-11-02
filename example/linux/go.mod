module github.com/Code-Hex/vz/example/linux

go 1.19

replace github.com/Code-Hex/vz/v2 => ../../

require (
	github.com/Code-Hex/vz/v2 v2.0.0-00010101000000-000000000000
	github.com/pkg/term v1.1.0
	golang.org/x/sys v0.1.0
)

require golang.org/x/mod v0.6.0 // indirect
