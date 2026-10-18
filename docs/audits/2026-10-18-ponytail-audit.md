# Ponytail Audit: FluxSend Backend

- Date: 2026-10-18
- Repository: `fluxsend-backend`
- Commit at audit time: `4c7d59e`
- Mode: Ponytail full
- Scope: whole repository

What this repo does: This is the Go backend for FluxSend, a file upload and sharing service. It keeps personal and workspace file metadata in Postgres, stores file bytes in S3, MinIO, or Google Cloud Storage, and serves a browser API plus an API-key CLI. It also handles OAuth and password login, share links, quotas, and admin endpoints.

Load assumed: a multi-tenant self-hosted deployment behind a TLS reverse proxy, with many users, large workspaces, 10 GB files, and real API-key use. Some findings need the enterprise plan or a large workspace before they hurt.

This audit changed no code.

## Must fix

1. **Folder delete can wipe another folder's records** (`internal/service/workspace_file.go:888`, `internal/repo/query/workspace_files.sql:45`)
   - **What this is:** Deleting a workspace folder removes every row whose path starts with that folder.
   - **Problem:** The folder path is put straight into a SQL `LIKE` pattern. Since folder names may contain `_` or `%`, deleting `/a_b` also matches `/axb/*`. The storage cleanup above only deletes the real prefix, so the sibling files lose their DB rows and their objects are left behind.
   - **Fix:** Escape `\`, `%`, and `_` in the path and add `ESCAPE '\'` to the query. Add one test with `/a_b` and `/axb`.
   - **If we skip it:** A routine folder delete silently destroys another folder's file list.

2. **Account deletion leaves files after the first 1,000** (`internal/cloud_storage/s3/s3_handler.go:425`)
   - **What this is:** S3 account deletion lists and deletes one page of objects under the user's prefix.
   - **Problem:** `ListObjectsV2` returns at most 1,000 keys by default, and the code never follows `ContinuationToken`. Enterprise plans allow far more than 1,000 files, so the extra objects stay after the account is gone.
   - **Fix:** Use the S3 paginator, or loop while `IsTruncated` is true, and delete every page before returning success.
   - **If we skip it:** "Delete my data" reports success while files remain in storage forever.

3. **Account deletion continues when storage cleanup fails** (`internal/service/user.go:104`)
   - **What this is:** The service logs a bucket-deletion error and deletes the user row anyway.
   - **Problem:** If S3, GCS, or MinIO is temporarily unavailable, the account is removed and the user can no longer retry. The objects are left with no owner and no clean way to find them again.
   - **Fix:** When `deleteStorageData` is true, return the storage error and keep the account. Delete the DB row only after storage cleanup succeeds.
   - **If we skip it:** A transient storage error turns a deletion request into permanent orphaned data.

4. **Deleting a workspace or admin-deleting a user leaves storage objects** (`internal/service/workspace.go:151`, `internal/service/admin.go:62`)
   - **What this is:** Workspace deletion removes DB rows through cascade. Admin user deletion removes only the user row.
   - **Problem:** Workspace files live under `<workspace_id>/...` in the shared bucket, and personal files under `<user_id>/...`. Neither prefix is deleted, and the cascade removes the only record of the keys.
   - **Fix:** Delete the storage prefix before the DB delete, and reuse one cleanup helper for both paths.
   - **If we skip it:** Deleted workspaces and admin-deleted users keep costing money and keep their data.

5. **Any logged-in user can write to another user's upload** (`internal/service/file.go:84`, `internal/api/upload_post.go:105`)
   - **What this is:** The private multipart part, complete, and abort handlers accept an upload ID and act on it.
   - **Problem:** The service never compares `upload.OwnerID` with the caller. Anyone who learns another user's upload ID from a support ticket, screen share, or log can upload parts into that user's file, complete it, or cancel it.
   - **Fix:** Pass the caller's user ID into the three service methods and reject when `upload.OwnerID` differs. Add one test for each handler.
   - **If we skip it:** Upload IDs become bearer tokens for other people's storage.

6. **Password login and registration have no rate limit** (`internal/api/auth.go:158`, `internal/api/auth.go:905`, `internal/service/password_auth.go:198`)
   - **What this is:** Both public endpoints run Argon2id, which uses 64 MiB per hash.
   - **Problem:** Neither path has a limiter. A modest flood of wrong passwords or random registrations can exhaust memory and CPU. The database already has `login_password` and `register_send` scopes that nothing uses.
   - **Fix:** Apply the existing `auth_rate_limits` table to login and registration, and cap concurrent Argon2 work.
   - **If we skip it:** One unauthenticated client can make the login service unavailable for everyone.

7. **Documented storage backends cannot handle the upload path clients use** (`internal/cloud_storage/gcs/gcs_handler.go:68`, `internal/cloud_storage/minio/minio_handler.go:407`, `fluxsend-frontend/src/api.ts:645`)
   - **What this is:** GCS implements only single-shot upload. MinIO implements only multipart upload.
   - **Problem:** The frontend always uses multipart, so every upload on GCS returns 500. The public `/files/upload` and workspace single-upload routes fail on MinIO. The docs still list all three providers as supported (`docs-site/config-reference/02-storage.md:5`).
   - **Fix:** Implement the missing method for each provider, or have the server advertise its upload capability and let the client use the working path.
   - **If we skip it:** GCS deployments cannot accept a single file, and MinIO API users hit a 500 on the documented endpoint.

8. **Every JSON response is missing `Content-Type`** (`pkg/lib.go:185`)
   - **What this is:** `WriteJSONResponse` calls `WriteHeader`, then `JSON` sets `Content-Type`.
   - **Problem:** Headers set after `WriteHeader` are ignored, so Go serves these as `text/plain`. Generated clients, caches, and anything that checks the content type see the wrong format.
   - **Fix:** Set `Content-Type` before `WriteHeader`. Add one handler test that asserts the header.
   - **If we skip it:** The API contract is wrong for every endpoint even though the body is JSON.

9. **One share request can exceed the daily share limit** (`internal/api/sharing_post.go:58`, `internal/service/share.go:145`, `internal/repo/query/plans.sql:64`)
   - **What this is:** The quota check counts shares that already exist, then the service inserts one share per file.
   - **Problem:** At 9 of 10 daily shares, a request with 100 files passes the check and creates 100 shares. The limit is bypassed by batching.
   - **Fix:** Check `existing + len(objects) > max` before inserting anything, in one transaction.
   - **If we skip it:** Free and paid share limits can be bypassed with one request.

10. **Behind a proxy, one user's reset cooldown blocks everyone** (`internal/api/api.go:197`, `internal/api/auth.go:813`)
    - **What this is:** The API uses `ClientIPFromRemoteAddr`, and the self-reset flow keys a 60-second IP cooldown on that value.
    - **Problem:** In the documented Docker, Traefik, and Nginx setup, every request has the proxy's address. User A requesting a reset makes user B receive 429 for the next minute.
    - **Fix:** Trust only configured proxy addresses and use their forwarded client IP. Keep the direct address when no proxy is configured.
    - **If we skip it:** Password reset becomes unusable for everyone after the first request.

## Should fix

11. **The last owner can demote themselves and lock the workspace** (`internal/api/workspaces_patch.go:156`)
    - **What this is:** An owner may change any member's role, including their own, to `admin`, `editor`, or `viewer`.
    - **Problem:** After that, no member has the `owner` role. Deleting the workspace requires `owner`, and assigning `owner` also requires `owner`, so the workspace is stuck.
    - **Fix:** Reject self-demotion when the caller is the only owner, or require another owner first.
    - **If we skip it:** A workspace can be left undeletable and unmanageable through the UI.

12. **API-key login scans and bcrypt-checks every active key** (`internal/repo/query/api_keys.sql:109`, `internal/api/auth_cli.go:55`)
    - **What this is:** The lookup is `WHERE key_hash = crypt($1, key_hash)`.
    - **Problem:** Postgres must run bcrypt for every active row. With 100 active keys, every API request does about 100 bcrypt checks; with thousands, requests take minutes.
    - **Fix:** Store a deterministic SHA-256 lookup hash in a unique column and compare it in one indexed lookup. The key is already 256-bit random.
    - **If we skip it:** The CLI API slows down as soon as a real customer has many keys.

13. **Every authenticated request does a storage bucket check** (`internal/api/auth.go:1372`, `internal/cloud_storage/s3/s3_handler.go:219`)
    - **What this is:** The auth middleware calls `CreateBucketIfNotExists` on every request.
    - **Problem:** On S3 that is a `HeadBucket` network call; on GCS and MinIO it is a bucket-stat call. It adds latency to every API request and doubles S3 request volume.
    - **Fix:** Call it only when a user is created or logs in, and retry on the first upload if the bucket is missing.
    - **If we skip it:** Every page load pays for a storage round trip.

14. **Quota checks race with the write they protect** (`internal/api/validate_classic_plan.go:21`, `internal/api/upload_post.go:71`)
    - **What this is:** Upload and workspace creation check the current count, then insert later.
    - **Problem:** Five parallel uploads at 19 of 20 files can all pass the check and finish at 24. The same pattern applies to storage quota, workspace count, and members.
    - **Fix:** Do the count and the insert in one transaction with a row lock, or enforce the limit at completion in a transaction.
    - **If we skip it:** Users can exceed file and storage limits by uploading in parallel.

15. **Batch delete drops DB rows after storage cleanup fails** (`internal/service/file.go:713`, `internal/cloud_storage/s3/s3_handler.go:355`)
    - **What this is:** Batch delete removes objects one by one under a single 10-second timeout, then deletes every DB row even if the storage call failed.
    - **Problem:** Large batches time out; the warning is logged and the rows disappear. The files are gone from the UI but still in storage.
    - **Fix:** Use S3 `DeleteObjects` in batches, and only delete DB rows whose object delete succeeded.
    - **If we skip it:** Storage costs keep growing for files users believe they deleted.

16. **Abandoned multipart uploads are never cleaned** (`internal/repo/query/file_uploads.sql:18`, `internal/service/file.go:471`)
    - **What this is:** Upload rows stay `uploading` forever if the client disappears.
    - **Problem:** The browser aborts only on explicit cancel. A network error or closed tab leaves S3 multipart parts and a DB row. Nothing lists or expires them.
    - **Fix:** Add a periodic job that aborts uploads older than a few hours and deletes their parts.
    - **If we skip it:** Incomplete uploads accumulate storage cost forever.

17. **Workspace invite deletion is not scoped to the workspace** (`internal/api/workspaces_delete.go:106`, `internal/repo/query/workspaces.sql:68`)
    - **What this is:** The handler checks the caller is an admin of the workspace in the query string, then deletes the invite by ID alone.
    - **Problem:** An admin of workspace A who learns an invite ID from workspace B can delete B's invite. The ID is random, but the authorization check is not tied to the row.
    - **Fix:** Add `workspace_id` to the delete query and pass the checked workspace ID.
    - **If we skip it:** A leaked invite ID becomes a cross-workspace delete.

18. **Risky delete and ownership code has no tests** (`internal/service/workspace_file_test.go`, `internal/service/user_test.go`)
    - **What this is:** Tests cover happy paths and some not-found paths.
    - **Problem:** There is no test for `_` or `%` folder deletion, upload ownership, S3 pagination, or storage failure during account deletion. The suite passes while findings 1-5 are broken.
    - **Fix:** Add one focused test per path. Each test should fail on the current code.
    - **If we skip it:** The next refactor can reintroduce the same data-loss bugs silently.

## Nice to have

19. **Workspace browsing loads the whole workspace** (`internal/service/workspace_file.go:615`)
    - **What this is:** Each tree request loads every file in the workspace, then loops over files and folders to sum sizes.
    - **Problem:** A workspace with 100 folders and 10,000 files does about 1,000,000 path checks per browse, even though the scoped SQL query already fetches the visible children.
    - **Fix:** Compute folder sizes in one pass over the loaded rows, or with a single SQL aggregate.
    - **If we skip it:** Large workspaces get slower every time a folder is opened.

20. **No-op middleware and unused helpers** (`internal/middleware/limits.go:11`, `internal/mappings/mappings.go:11`)
    - **What this is:** `EnforceFileUploadLimits` only copies the context and is never registered. `MapBucketDataToDBFormat` and `FindMissingFilesFromDB` have no callers.
    - **Problem:** Dead code has to be read and maintained, and the middleware name suggests a limit that does not exist.
    - **Fix:** Delete both files. They are 77 lines in total.
    - **If we skip it:** Readers keep assuming there is an upload limiter and a bucket-to-DB mapper.

Smaller findings left out: the SES mailer sends a literal `Subject:` prefix in the subject, API keys never update `last_used_at`, public downloads run three token queries, password reset and registration leak whether an email exists, sharing a missing file returns 200 with an empty list, and admin basic auth compares credentials with plain `!=`. That is 6 more.

Verdict: Not healthy. Fix findings 1-10 first, especially 1-4, before trusting delete and account flows with real user data.

Lean: -77 lines, -0 dependencies possible.

Not checked: `s3-migrate/`, the docs site, and the full frontend behavior were not audited. `go build ./...` and `go vet ./...` pass. `go test ./...` passes except `internal/cloud_storage/minio`, which could not open a local test listener in this sandbox. No live Postgres or object-storage integration run was possible here.
