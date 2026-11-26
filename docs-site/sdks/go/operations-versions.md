---
title: Go SDK operations and versions
description: FluxSend Go SDK operation coverage and compatibility.
---

# Operations & versions

The Go SDK follows the FluxSend CLI API contract and exposes typed methods for
every documented operation.

## Coverage

| Surface | Count | Authentication |
| --- | ---: | --- |
| API-key-backed operations | 60 | Private or workspace API key |
| Public sharing operations | 3 | No API key |
| **Total** | **63** | |

Coverage is verified in CI against the backend OpenAPI document and the stable
operation-name manifest. The three public operations are `GetPublicShareInfo`,
`ResolvePublicShare`, and `DownloadPublicShare`.

## Operation groups

| Area | Examples | API reference |
| --- | --- | --- |
| API keys | `CreatePrivateAPIKey`, `ListWorkspaceAPIKeys`, `DeletePrivateAPIKey` | [API reference](../../api/index.md) |
| Files, folders, and sharing | `UploadFile`, `GetFileTree`, `ShareFiles`, `UploadFileSession` | [API reference](../../api/index.md) |
| Users | `GetCurrentUserData`, `GetUserStatistics`, `CreatePrivateDownloadToken` | [API reference](../../api/index.md) |
| Workspaces | `ListWorkspaces`, `CreateWorkspace`, `UploadWorkspaceFileSession`, `GetWorkspaceQuota` | [API reference](../../api/index.md) |

Use the [interactive API reference](../../api/index.md) for exact request
parameters, response schemas, and status codes. The SDK guide deliberately does
not duplicate that generated schema.

## Method conventions

- API-key-backed operations are methods on `*Client`.
- Method names follow the stable API operation names.
- Every network method accepts `context.Context` as its first argument.
- Typed request bodies use SDK-owned `...Request` values.
- Successful responses are decoded into Go values; callers do not unwrap the
  FluxSend response envelope.
- Path parameters are escaped by the SDK.
- Streaming transfers use the helpers described in
  [Files & transfers](files-transfers.md).

## Versioning and compatibility

The latest published module release is
[`v0.1.1`](https://github.com/tscrond/fluxsend-sdk/releases/tag/v0.1.1).

- The SDK is versioned independently from the FluxSend backend.
- `v0.x` releases may introduce breaking exported API changes; each release is
  documented in the [changelog](https://github.com/tscrond/fluxsend-sdk/blob/main/CHANGELOG.md).
- Starting with `v1.0.0`, backwards-incompatible exported API changes require a
  major version.
- Deprecated APIs remain available for at least one minor release when feasible.

## Known limits

- The SDK is Go-only.
- Chunked-upload resume is not persisted across processes.
- Browser OAuth and CLI credential storage are outside the SDK.
- The SDK does not package or install the FluxSend CLI.

The SDK owns HTTP communication, authentication headers, typed response decoding,
retry safety, and streaming transfer behavior. The CLI owns command parsing,
profiles, interactive prompts, output formatting, and process exit codes.
