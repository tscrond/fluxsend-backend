# Use Cases

## Personal Cloud Storage

Use FluxSend as a self-hosted or SaaS alternative to Dropbox or Google Drive. Upload personal files, organize them into folders, and access them from anywhere through the web interface. Attach notes to files for context, and manage your storage with per-user quotas enforced by your chosen plan.

## Secure File Sharing

Share files with specific people or create public share links with optional password protection and expiry dates. FluxSend supports two sharing modes:

- **Targeted sharing** — Share files directly with another user by email. The recipient receives an in-app notification and optional email alert. Shares can be revoked at any time.
- **Quick sharing** — Generate a public link for anyone, even non-users. Protect the link with a password (bcrypt-hashed, with brute-force protection that auto-deletes after 5 failed attempts).

## Team Collaboration with Workspaces

Create shared workspaces where team members can upload, manage, and collaborate on files in a shared storage space. Workspaces support role-based access control:

| Role | Permissions |
|---|---|
| Owner | Full control — manage members, roles, invites, delete workspace |
| Admin | Full control over files and members |
| Editor | Upload and manage their own files within the workspace |
| Viewer | Read-only access to workspace files |

Invite team members by email, accept or reject invitations, and manage member roles as your team grows.

## Automated File Operations via API

Generate scoped API keys to integrate FluxSend into your workflows and automations. API keys support fine-grained permission scopes:

- `private_files:read / write / delete / share`
- `workspaces:read / write / delete`
- `workspaces:members:read / manage`
- `workspaces:invites:manage`
- `workspaces:files:read / write / delete`

Use the REST API for CI/CD pipelines, backup automation, or building custom integrations on top of FluxSend.

## Self-Hosted File Infrastructure

Deploy FluxSend on your own infrastructure for complete control over your data. Choose your storage backend:

- **Google Cloud Storage** — Per-user buckets with signed URLs and public-access prevention
- **AWS S3** — Single bucket with per-user prefixes and presigned URLs
- **CloudFront CDN** — Optional signed-URL CDN delivery on top of S3

Pair FluxSend with your existing OAuth provider (Google or GitHub) for authentication, and route email notifications through SES or SMTP.
