---
title: Go SDK quickstart
description: Install FluxSend Go SDK and make your first request.
---

# Quickstart

This guide takes you from an empty Go module to a typed FluxSend API call. You
need Go 1.27.4 or newer, a FluxSend deployment URL, and an API key for operations
that access your account or a workspace.

## Install

```sh
go get github.com/tscrond/fluxsend-sdk@v0.1.1
```

The public package name is `fluxsend`:

```go
import fluxsend "github.com/tscrond/fluxsend-sdk"
```

## Create an API key

Open [FluxSend API Keys](https://fluxsend.app/api-keys) to create a personal or
workspace key. Keep the key in server-side configuration, such as an environment
variable or secrets manager. Do not expose it in browser code or commit it to a
repository.

Public sharing operations are the exception: `GetPublicShareInfo`,
`ResolvePublicShare`, and `DownloadPublicShare` can be called without an API key.

## Make your first request

Create a client with the deployment base URL and an API key. Use a context with
a deadline so the request can be cancelled or bounded:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	fluxsend "github.com/tscrond/fluxsend-sdk"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := fluxsend.NewClient(fluxsend.Config{
		BaseURL: "https://fluxsend.app",
		APIKey:  os.Getenv("FLUXSEND_API_KEY"),
	})
	if err != nil {
		fmt.Println(err)
		return
	}

	workspaces, err := client.ListWorkspaces(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("found %d workspaces\n", len(workspaces))
}
```

`NewClient` validates the configuration. A client without an API key can still
call public sharing operations, but an API-key-backed operation fails locally
with an authentication error.

## Select an endpoint

The SDK method names follow the API operation names:

| Operation | SDK method |
| --- | --- |
| List workspaces | `Client.ListWorkspaces` |
| Create a workspace | `Client.CreateWorkspace` |
| Upload a personal file | `Client.UploadFile` |
| Start a chunked upload | `Client.UploadFileSession` |
| Open a private download | `Client.DownloadPrivateFile` |

All network operations accept `context.Context` as their first argument. The SDK
escapes path parameters and returns typed response values instead of the raw
FluxSend response envelope.

!!! tip "Compile-tested examples"
    The quickstart is backed by `ExampleNewClient` and
    `ExampleClient_CreateWorkspace` in the
    [SDK repository](https://github.com/tscrond/fluxsend-sdk/blob/main/example_test.go).

## Next steps

- [Configure the client and API keys](configuration-authentication.md).
- [Handle typed errors and cancellation](errors-reliability.md).
- [Upload and download files](files-transfers.md).
- [Browse all supported operations and versions](operations-versions.md).
