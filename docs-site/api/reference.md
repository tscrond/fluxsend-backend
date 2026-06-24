# Endpoint Reference

This page is generated from the current OpenAPI 3 document. Regenerate it with `make swagger`.

## Api Keys

| Method | Path | Summary | Description |
| --- | --- | --- | --- |
| GET | /api/api_keys/private/list | List private API keys | Returns all private API keys created by the authenticated user. |
| DELETE | /api/api_keys/private/delete | Delete private API key | Deletes a private API key owned by the authenticated user. |
| POST | /api/api_keys/private/create | Create private API key | Creates a private API key for the current user. |
| GET | /api/api_keys/{workspace_id}/list | List workspace API keys | Returns the API keys created for the selected workspace. |
| DELETE | /api/api_keys/{workspace_id}/delete | Delete workspace API key | Deletes a workspace-scoped API key by ID. |
| POST | /api/api_keys/{workspace_id}/create | Create workspace API key | Creates a workspace-scoped API key for users allowed to manage workspace API keys. |

## Downloads

| Method | Path | Summary | Description |
| --- | --- | --- | --- |
| GET | /api/d/{token} | Download public share | Resolves a public share token and either redirects or proxies the file content. |
| GET | /api/d/private/{token} | Download private file | Resolves a private download token for the authenticated file owner and redirects to the download URL. |

## Files

| Method | Path | Summary | Description |
| --- | --- | --- | --- |
| POST | /api/folders/move | Move folder | Moves a personal folder to another path. |
| GET | /api/folders | List folders | Returns the folders that exist at the supplied personal path. |
| DELETE | /api/folders | Delete folder | Deletes a personal folder and optionally all nested files and folders when recursive is true. |
| POST | /api/files/upload | Upload a file | Uploads a file to the current user's personal storage and optionally stores it in a folder. |
| GET | /api/files/tree | Get file tree | Returns the authenticated user's personal file tree for a given path prefix. |
| POST | /api/files/move | Move file | Moves a personal file from one path to another. |
| DELETE | /api/files/delete/batch | Delete multiple files | Deletes several personal files in one request. |
| DELETE | /api/files/delete | Delete a file | Deletes a file from the authenticated user's personal storage. |
| GET | /api/files/{checksum}/note | Get file note | Returns the note content for a file checksum. |
| PUT | /api/files/{checksum}/note | Edit file note | Updates or creates the note for a file checksum. |

## Sharing

| Method | Path | Summary | Description |
| --- | --- | --- | --- |
| GET | /api/files/shared_by_user | List outgoing shares | Returns all files that the current user has shared with others. |
| POST | /api/files/share | Share files | Shares one or more personal files with another user, optionally emailing the recipient. |
| GET | /api/files/received/unseen_count | Count unseen received files | Returns how many received shares have not yet been marked as seen. |
| POST | /api/files/received/mark_seen | Mark received share as seen | Marks a received share as seen by token. |
| GET | /api/files/received | List received shares | Returns all files shared with the current user. |
| POST | /api/files/quick_share | Create quick share | Creates a public share token for a personal file. |
| POST | /api/share/revoke/{token} | Revoke share | Revokes a previously created public share token. |
| POST | /api/share/resolve/{token} | Resolve public share | Resolves a public share token and returns a signed download URL. |
| GET | /api/share/info/{token} | Get public share info | Returns public metadata for a share token without requiring authentication. |

## User

| Method | Path | Summary | Description |
| --- | --- | --- | --- |
| GET | /api/user/stats | Get user statistics | Returns aggregate file, share, and workspace statistics for the current user. |
| POST | /api/user/private/download_token | Create private download token | Generates a download token for a private file name. |
| GET | /api/user/data | Get current user data | Returns the authenticated user's profile and plan details. |
| GET | /api/user/bucket | Get user bucket data | Returns the current user's bucket metadata and usage details. |
| DELETE | /api/user/account/delete | Delete account | Deletes the authenticated user's account and optionally keeps or removes bucket data. |

## Workspace Files

| Method | Path | Summary | Description |
| --- | --- | --- | --- |
| POST | /api/workspaces/{workspace_id}/files/upload | Upload workspace file | Uploads a file into a workspace folder for users with write access. |
| GET | /api/workspaces/{workspace_id}/files/tree | Get workspace file tree | Returns the file tree for a workspace path visible to the caller. |
| PATCH | /api/workspaces/{workspace_id}/files/move | Move workspace file | Moves a file to another path within the same workspace. |
| POST | /api/workspaces/{workspace_id}/files/mkdir | Create workspace folder | Creates a new folder inside the selected workspace. |
| PATCH | /api/workspaces/{workspace_id}/files/folder/move | Move workspace folder | Moves a folder to a new location inside the workspace. |
| DELETE | /api/workspaces/{workspace_id}/files/folder/delete | Delete workspace folder | Deletes a workspace folder when the caller has write access. |
| GET | /api/workspaces/{workspace_id}/files/download | Download workspace file | Creates a signed download URL for a workspace file and redirects or proxies based on mode. |
| DELETE | /api/workspaces/{workspace_id}/files/delete | Delete workspace file | Deletes a workspace file when the caller has write access. |

## Workspaces

| Method | Path | Summary | Description |
| --- | --- | --- | --- |
| PATCH | /api/workspaces/rename | Rename workspace | Updates the name and optionally the slug of a workspace owned by the current user. |
| PATCH | /api/workspaces/members/role | Change member role | Updates the role of a workspace member when the caller has permission to manage members. |
| DELETE | /api/workspaces/members/remove | Remove workspace member | Removes a member from a workspace when the caller has permission. |
| GET | /api/workspaces/members | List workspace members | Returns the members of a workspace when the caller has access to it. |
| GET | /api/workspaces/list | List workspaces | Returns all workspaces visible to the authenticated user. |
| DELETE | /api/workspaces/invites/reject | Reject workspace invite | Rejects a workspace invite using its token. |
| GET | /api/workspaces/invites/mine | List my workspace invites | Returns all workspace invites associated with the authenticated user. |
| DELETE | /api/workspaces/invites/delete | Delete workspace invite | Deletes a pending workspace invite when the caller can manage the workspace. |
| POST | /api/workspaces/invites/create | Create workspace invite | Creates a workspace invite for a specific email and role. |
| POST | /api/workspaces/invites/accept | Accept workspace invite | Accepts a workspace invite token for the authenticated user. |
| GET | /api/workspaces/invites | List workspace invites | Returns pending invites for a workspace when the caller is allowed to view them. |
| DELETE | /api/workspaces/delete | Delete workspace | Deletes a workspace owned by the authenticated user. |
| POST | /api/workspaces/create | Create workspace | Creates a new workspace using the authenticated user's account. |
| GET | /api/workspaces/{workspace_id}/quota | Get workspace quota | Returns storage, file, folder, member, and API key quota information for a workspace. |

