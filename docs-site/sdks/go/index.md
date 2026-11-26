---
title: FluxSend Go SDK
description: Install the FluxSend Go SDK and build typed file-transfer and workspace integrations.
---

# FluxSend Go SDK

Use the FluxSend Go SDK to add file uploads, downloads, sharing and workspace
automation to Go services. The SDK handles authentication, request encoding,
response decoding, retries and typed errors.

<div class="docs-callout">

**Go 1.27.4 or newer is required.** The current SDK release is `v0.1.1`.

</div>

## Install

```sh
go get github.com/tscrond/fluxsend-sdk@v0.1.1
```

The public package name is `fluxsend`:

```go
import fluxsend "github.com/tscrond/fluxsend-sdk"
```

## Send your first request

Create a client with the deployment URL and an API key, then call a typed
operation. Always pass a context so requests can be cancelled or bounded:

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

client, err := fluxsend.NewClient(fluxsend.Config{
	BaseURL: "https://fluxsend.app",
	APIKey:  os.Getenv("FLUXSEND_API_KEY"),
})
if err != nil {
	return err
}

workspaces, err := client.ListWorkspaces(ctx)
if err != nil {
	return err
}
```

Keep API keys in server-side configuration or a secrets manager. Do not embed
them in browser code.

## What it covers

The SDK exposes every operation in the FluxSend CLI API contract:

| Surface | Count | Authentication |
| --- | ---: | --- |
| API-key-backed operations | 60 | Private or workspace API key |
| Public sharing operations | 3 | No API key |
| **Total** | **63** | |

Operation methods use stable names such as `ListWorkspaces`,
`UploadFileSession` and `DownloadPrivateFile`. Request bodies use typed SDK
models, and successful responses are decoded into typed Go values.

## Choose a guide

<div class="fs-cards">
  <a class="fs-card" href="quickstart/">
    <span class="fs-card__title">Quickstart</span>
    <span class="fs-card__desc">Install the module, create a client and make your first typed request.</span>
    <span class="fs-card__more">Get started</span>
  </a>
  <a class="fs-card" href="configuration-authentication/">
    <span class="fs-card__title">Configuration &amp; authentication</span>
    <span class="fs-card__desc">Configure hosted or self-hosted deployments, key scopes and HTTP clients.</span>
    <span class="fs-card__more">Configure the client</span>
  </a>
  <a class="fs-card" href="errors-reliability/">
    <span class="fs-card__title">Errors &amp; reliability</span>
    <span class="fs-card__desc">Classify failures, honor rate limits and handle cancellation safely.</span>
    <span class="fs-card__more">Handle failures</span>
  </a>
  <a class="fs-card" href="files-transfers/">
    <span class="fs-card__title">Files &amp; transfers</span>
    <span class="fs-card__desc">Stream direct uploads, run chunked transfers and download files safely.</span>
    <span class="fs-card__more">Move files</span>
  </a>
  <a class="fs-card" href="operations-versions/">
    <span class="fs-card__title">Operations &amp; versions</span>
    <span class="fs-card__desc">Map all 63 operations and understand the current compatibility policy.</span>
    <span class="fs-card__more">Explore coverage</span>
  </a>
</div>

## Known limits

- The SDK is Go-only.
- `v0.x` releases may contain breaking changes before `v1.0.0`.
- Chunked-upload resume is not persisted across processes.
- Browser OAuth and CLI credential storage are outside the SDK.

See [operations and versions](operations-versions.md) for the complete coverage
and compatibility policy, or browse the generated [API reference](../../api/index.md)
for exact endpoint parameters and response schemas.
