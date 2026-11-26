---
title: Go SDK files and transfers
description: Upload and download files with the FluxSend Go SDK.
---

# Files & transfers

Use direct uploads for ordinary files and multipart sessions for large files that
benefit from bounded concurrency, progress, and resumable parts within one
process.

## Choose an upload path

| Workflow | SDK entry point | Best for |
| --- | --- | --- |
| Direct personal upload | `UploadFile` | Files that fit comfortably in one request |
| Direct workspace upload | `UploadWorkspaceFile` | Files uploaded into a bound workspace |
| Chunked personal upload | `UploadFileSession` | Large private files, progress, and bounded concurrency |
| Chunked workspace upload | `UploadWorkspaceFileSession` | Large files inside a bound workspace |

Direct uploads accept any `io.Reader`. Chunked uploads require an
`io.ReaderAt`, an accurate size, and a non-empty file name. Upload and download
operations always require a context.

## Direct upload

=== "Personal"

    ```go
    file, err := os.Open("report.pdf")
    if err != nil {
    	return err
    }
    defer file.Close()

    uploadedName, err := client.UploadFile(ctx, fluxsend.UploadFileRequest{
    	Contents: file,
    	FileName: "report.pdf",
    	Folder:   "documents",
    })
    ```

=== "Workspace"

    ```go
    file, err := os.Open("report.pdf")
    if err != nil {
    	return err
    }
    defer file.Close()

    files, err := client.UploadWorkspaceFile(ctx, workspaceID, fluxsend.UploadWorkspaceFileRequest{
    	Contents: file,
    	FileName: "report.pdf",
    	Folder:   "documents",
    })
    ```

The direct upload operations stream the request body. They do not buffer the
whole file in memory.

## Chunked upload

Chunked uploads require random access to the source:

```go
file, err := os.Open("archive.zip")
if err != nil {
	return err
}
defer file.Close()

info, err := file.Stat()
if err != nil {
	return err
}

uploadCtx, cancel := context.WithCancel(ctx)
defer cancel()

completed, err := client.UploadFileSession(uploadCtx, fluxsend.UploadSessionRequest{
	File:        file,
	FileName:    info.Name(),
	Size:        info.Size(),
	ContentType: "application/zip",
	Folder:      "backups",
	Concurrency: 4,
	Progress: func(progress fluxsend.UploadProgress) {
		log.Printf("%d / %d bytes", progress.UploadedBytes, progress.TotalBytes)
	},
})
```

For workspace storage, call `UploadWorkspaceFileSession` and pass the workspace
ID before the request:

```go
completed, err := client.UploadWorkspaceFileSession(uploadCtx, workspaceID, request)
```

Canceling `uploadCtx` stops new parts and interrupts in-flight requests. The SDK
then attempts to abort the server-side upload session. Abort cleanup is
best-effort; failed uploads may still require administrative cleanup if the
deployment cannot be reached.

!!! warning "No cross-process resume"
    Completed parts are not persisted to disk. A new process must initialize a
    new upload session and upload the file again.

## Download files

Downloads return a `*fluxsend.Download`. The caller owns its response body and
must close it:

=== "Private file"

    ```go
    download, err := client.DownloadPrivateFile(ctx, downloadToken, "download")
    if err != nil {
    	return err
    }
    defer download.Body.Close()

    if _, err := io.Copy(destination, download.Body); err != nil {
    	return err
    }
    ```

=== "Public share"

    ```go
    download, err := client.DownloadPublicShare(ctx, shareToken, "download")
    if err != nil {
    	return err
    }
    defer download.Body.Close()

    if _, err := io.Copy(destination, download.Body); err != nil {
    	return err
    }
    ```

=== "Workspace file"

    ```go
    download, err := client.DownloadWorkspaceFile(ctx, workspaceID, fileID, "download")
    if err != nil {
    	return err
    }
    defer download.Body.Close()

    if _, err := io.Copy(destination, download.Body); err != nil {
    	return err
    }
    ```

Inspect read and close errors separately when the destination is durable. Do not
log signed storage URLs or downloaded file contents.

## Failure handling

- Validate file name and positive size before starting a chunked upload.
- Keep the source available until the session completes because part uploads use
  `ReaderAt`.
- Use the context to stop slow or abandoned transfers.
- Close every download body, including bodies you do not fully consume.
- Handle authentication and scope errors before retrying a transfer.

The transfer examples in
[`example_test.go`](https://github.com/tscrond/fluxsend-sdk/blob/main/example_test.go)
are compiled by the SDK test suite.
