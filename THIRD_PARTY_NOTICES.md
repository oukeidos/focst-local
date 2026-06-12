# Third-Party Notices

`focst-local` is a local-first subtitle translation CLI. This notice summarizes
third-party code, runtimes, services, and model files that are relevant to this
repository.

## Go Modules

The CLI is built from the Go module graph recorded in `go.mod` and `go.sum`.
Direct Go dependencies currently include:

- `github.com/asticode/go-astisub`
- `github.com/google/uuid`
- `github.com/rivo/uniseg`
- `github.com/spf13/cobra`
- `github.com/spf13/pflag`
- `github.com/zalando/go-keyring`
- `golang.org/x/sys`
- `golang.org/x/term`

Transitive Go dependencies currently include:

- `al.essio.dev/pkg/shellescape`
- `github.com/asticode/go-astikit`
- `github.com/asticode/go-astits`
- `github.com/danieljoos/wincred`
- `github.com/godbus/dbus/v5`
- `github.com/inconshreveable/mousetrap`
- `github.com/stretchr/testify`
- `golang.org/x/net`
- `golang.org/x/text`

The authoritative dependency list is the current Go module graph. Use
`go list -m all`, `go.mod`, and `go.sum` when auditing a specific build. Each
third-party module remains under its own license.

## External Runtime And Services

`focst-local` talks to `llama-server` through llama.cpp's OpenAI-compatible HTTP
API. llama.cpp is not bundled with this repository unless you package it
separately, and its own license applies.

Local model files, including GGUF files used with llama.cpp, are not bundled
with this repository. Users and distributors are responsible for complying with
the licenses and terms for the models they choose to run.

The optional `names` workflow can call OpenAI services to generate a character
and proper-name mapping file. OpenAI is not used for local subtitle translation,
and no OpenAI SDK is vendored by this repository. If the `names` workflow is
used, OpenAI's applicable terms, pricing, rate limits, and data policies apply
to that workflow.

## Upstream Project Reference

`focst-local` keeps and adapts parts of the subtitle-processing workflow from
FoCST for a local llama.cpp translation pipeline. See the project README for the
upstream FoCST reference.

## Redistribution

Before publishing formal binary releases, regenerate or verify full third-party
license texts from the exact dependency graph used for the release and include
any notices required by those licenses.
