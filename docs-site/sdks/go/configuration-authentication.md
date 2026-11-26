---
title: Go SDK configuration and authentication
description: Configure the FluxSend Go client and API-key access.
---

# Configuration & authentication

The SDK is configured explicitly. There is no global client and credentials are
never read from implicit CLI profiles.

## Client configuration

```go
client, err := fluxsend.NewClient(fluxsend.Config{
	BaseURL: "https://fluxsend.app",
	APIKey:  os.Getenv("FLUXSEND_API_KEY"),
})
```

| Field | Behavior |
| --- | --- |
| `BaseURL` | Absolute HTTP or HTTPS deployment URL, without a query or fragment |
| `APIKey` | Sent as `X-API-Key` only on operations that require authentication |
| `HTTPClient` | Optional client for proxies, TLS configuration, transport tuning, and timeouts |
| `UserAgent` | Optional request identifier; defaults to a versioned FluxSend SDK value |
| `AllowInsecureHTTP` | Explicitly permits plain HTTP for local development |

When `HTTPClient` is omitted, the SDK creates a client with a 30-second timeout.
Supply your own client when the service needs a different timeout or transport.
A `Client` is safe for concurrent use after construction.

## Hosted and self-hosted deployments

Use the origin of the FluxSend deployment:

```go
client, err := fluxsend.NewClient(fluxsend.Config{
	BaseURL: "https://files.example.com",
	APIKey:  os.Getenv("FLUXSEND_API_KEY"),
})
```

If FluxSend is mounted below a path prefix, include that prefix in `BaseURL`:

```go
client, err := fluxsend.NewClient(fluxsend.Config{
	BaseURL: "https://example.com/fluxsend",
	APIKey:  os.Getenv("FLUXSEND_API_KEY"),
})
```

Do not append `/api` to `BaseURL`; SDK operations add their own API paths. Queries
and fragments are rejected during client construction.

HTTPS is required by default. Plain HTTP is rejected unless
`AllowInsecureHTTP: true` is set, which should normally be limited to local
development:

```go
client, err := fluxsend.NewClient(fluxsend.Config{
	BaseURL:           "http://localhost:8080",
	APIKey:            "local-development-key",
	AllowInsecureHTTP: true,
})
```

## API key types

FluxSend has three authentication surfaces:

| Surface | Credential | Use |
| --- | --- | --- |
| Private API key | Personal key | Access the key owner's private files, shares, account data, and authorized workspaces |
| Workspace API key | Workspace-bound key | Access only the bound workspace and the scopes granted to the key |
| Public sharing operations | None | Read public share information, resolve a share, or download a public file |

The server enforces both key scopes and workspace bindings. The SDK sends the key
only to the configured deployment origin and removes it when an HTTP redirect
crosses to another host. The SDK does not retry non-idempotent mutations
implicitly.

## Supply an HTTP client

Use `HTTPClient` to control proxies, TLS, connection reuse, or a per-client
timeout:

```go
httpClient := &http.Client{
	Timeout: 45 * time.Second,
}

client, err := fluxsend.NewClient(fluxsend.Config{
	BaseURL:    "https://fluxsend.app",
	APIKey:     os.Getenv("FLUXSEND_API_KEY"),
	HTTPClient: httpClient,
})
```

Pass a timeout-aware context to every SDK operation as well. The context handles
per-request cancellation, while the HTTP client timeout protects the underlying
transport.

## Keep credentials out of user-visible data

- Load API keys from environment variables or a secrets manager.
- Do not print request headers, signed download URLs, or complete file contents.
- Do not ship private or workspace API keys in browser, mobile, or other
  user-controlled client code.
- Rotate and revoke keys through FluxSend when they may have been exposed.
