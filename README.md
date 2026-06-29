# apify-plugin

Public SDK and developer tooling for Apify custom plugins.

## Layout

| Path | Purpose |
| ---- | ------- |
| `plugin/` | Go SDK types and HTTP JSON runtime helper. |
| `proto/` | Stable external process plugin protocol definition. |
| `cmd/apify-plugin/` | Developer CLI for scaffolding, validation, local build, and packaging. |
| `examples/plugins/` | Example custom plugins using the public SDK. |

## Quick Start

```bash
go run ./cmd/apify-plugin init custom-header --dir /tmp/custom-header
go run ./cmd/apify-plugin validate --manifest /tmp/custom-header/apify-plugin.yaml
go run ./cmd/apify-plugin pack --dir /tmp/custom-header --output /tmp/custom-header.tar.gz
```

For plugin projects, import:

```go
import sdk "github.com/apifyhost/apify-plugin/plugin"
```

## Upload Model

Apify accepts custom plugin source archives only. The platform validates uploaded source,
builds the executable artifact in a controlled environment, then loads the approved artifact
in the data plane.
