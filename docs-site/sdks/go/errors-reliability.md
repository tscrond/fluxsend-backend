---
title: Go SDK errors and reliability
description: Handle FluxSend Go SDK errors, retries and cancellation.
---

# Errors & reliability

The SDK separates local configuration errors, transport failures, context
cancellation, and API responses. Use standard Go error inspection so callers can
handle each case deliberately.

## Error categories

| Sentinel | Meaning |
| --- | --- |
| `fluxsend.ErrAuthentication` | Missing or rejected API key |
| `fluxsend.ErrAuthorization` | Valid credential without the required scope or binding |
| `fluxsend.ErrValidation` | Invalid request data |
| `fluxsend.ErrNotFound` | Requested resource does not exist |
| `fluxsend.ErrConflict` | Request conflicts with current state |
| `fluxsend.ErrGone` | Share or resource is no longer available |
| `fluxsend.ErrRateLimit` | The deployment rate-limited the request |
| `fluxsend.ErrServer` | The deployment returned a server error |
| `fluxsend.ErrTransport` | Network or transport failure |
| `fluxsend.ErrAPI` | Other API, response-envelope, or decoding failure |

`APIError` carries the HTTP status, safe FluxSend message, bounded response data,
and parsed retry delay:

```go
workspaces, err := client.ListWorkspaces(ctx)
if errors.Is(err, fluxsend.ErrAuthentication) {
	return errors.New("check FLUXSEND_API_KEY and its binding")
}

var apiErr *fluxsend.APIError
if errors.As(err, &apiErr) {
	log.Printf("FluxSend status=%d message=%s", apiErr.StatusCode, apiErr.Message)
}

if errors.Is(err, fluxsend.ErrRateLimit) {
	time.Sleep(apiErr.RetryAfter)
}
```

Do not include `X-API-Key`, signed URLs, or response bodies containing user file
data in logs.

## Retries and rate limits

Read-only operations may retry transient transport failures and rate-limit
responses. Retries are bounded to two additional attempts, use bounded backoff,
and honor `Retry-After` when the deployment provides it.

Mutations are not retried implicitly because replaying an upload, delete, or
share creation can duplicate or destroy data. If a mutation fails, inspect the
error and decide explicitly whether replaying the operation is safe.

The SDK exposes the parsed delay through `APIError.RetryAfter`:

```go
var apiErr *fluxsend.APIError
if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
	timer := time.NewTimer(apiErr.RetryAfter)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		// Retry only after confirming this operation is safe to repeat.
	}
}
```

## Context and cancellation

Every network operation accepts a context. Cancellation stops new requests and
interrupts in-flight HTTP work:

```go
ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
defer cancel()

workspaces, err := client.ListWorkspaces(ctx)
if errors.Is(err, context.DeadlineExceeded) {
	// The request exceeded the caller's deadline.
}
```

For chunked uploads, cancellation also stops new parts. The SDK makes a
best-effort abort request for the server-side upload session. That cleanup uses a
short context independent of the already-cancelled request.

## Response safety

- Error bodies are bounded before decoding.
- JSON success bodies have a size limit; use streaming download operations for
  file content instead of JSON operations.
- Download bodies stay open until the caller closes them.
- The SDK does not log API keys or file contents.

The package-level examples in
[`example_test.go`](https://github.com/tscrond/fluxsend-sdk/blob/main/example_test.go)
compile these error and cancellation patterns as part of CI.
