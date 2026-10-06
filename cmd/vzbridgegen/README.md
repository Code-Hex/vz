# Regenerating bridge bindings

The generators read committed metadata to produce the Go files in
`internal/vzbridge` without querying the SDK or host OS. Application builds do not
run the generators.

Regeneration works without Xcode. On macOS,
`go generate ./internal/vzbridge` runs both generators.

CI checks that the supported Go versions produce identical output on Linux.
Run from the repository root:

```sh
go run ./cmd/vzbridgegen
go run ./internal/vzbridge/nativegen -input cmd/vzbridgegen/metadata/native.json -header internal/vzbridge/native.h -output internal/vzbridge/native_bindings.go
git diff --exit-code -- internal/vzbridge
```

The macOS CI matrix checks extraction and runtime compatibility through Clang
fixture tests and runtime ABI tests.

## Refreshing the inputs

Refresh metadata when adopting a new SDK or macOS release, changing extraction
rules, or editing `native.h`. Extraction requires macOS and Xcode. Run from the
repository root:

```sh
go run ./cmd/vzbridgegen -extract
go run ./internal/vzbridge/nativegen -extract -input cmd/vzbridgegen/metadata/native.json -header internal/vzbridge/native.h
```

`-extract` updates metadata only. Then regenerate the bindings with the commands
above and run the generator tests:

```sh
go test ./cmd/vzbridgegen ./internal/vzbridge/nativegen
```

The generators check hashes of the extractor source and `native.h`. Even comment
or formatting changes to extractor source require refreshing metadata. Changes to
code emitters do not.

Runtime metadata comes from one host and is used for both architectures. It can
miss methods available only on the other architecture.

Review metadata changes for removed APIs, types, availability, and ownership. Keep
extracted metadata and generated code in separate commits from handwritten
changes.
