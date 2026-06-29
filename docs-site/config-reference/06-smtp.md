# Email (SMTP / SES)

FluxSend sends email notifications when files are shared. Two providers are implemented: **SMTP** (standard) and **AWS SES**.

---

## Provider selection

The email provider is **hardcoded** to `"standard"` in `cmd/bootstrap.go`. To switch to SES, change the string to `"ses"` and rebuild. Unlike `STORAGE_PROVIDER`, there is no env var for this. Standard email provider is enough to support most popular mailer setups.

---

## Environment variables

### SMTP (standard)

| Variable | Description | Required |
|---|---|---|
| `SMTP_HOST` | SMTP server hostname | Yes |
| `SMTP_PORT` | SMTP server port (e.g. `587`) | Yes |
| `SMTP_USERNAME` | SMTP auth username | Yes |
| `SMTP_PASSWORD` | SMTP auth password | Yes |
| `MAIL_FROM` | Sender email address | Yes |

Uses `smtp.PlainAuth` over STARTTLS (port 587). Implicit TLS (port 465) is **not supported**.

### AWS SES

| Variable | Description | Required |
|---|---|---|
| `AWS_REGION` | AWS region for SES | Yes |
| `AWS_ACCESS_KEY_ID` | AWS access key | Yes |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key | Yes |
| `MAIL_FROM` | Sender email address | Yes |

Sends HTML-only emails (no plain-text alternative).

---

## Email template

A single HTML template is used at `internal/mailservice/templates/share.html`:

- Branded header with "FluxSend" title
- Lists shared files with per-file download button
- Expiry warning banner
- Responsive email design

**Template data:**

| Field | Description |
|---|---|
| `Files` | List of `{FileName, DownloadURL}` |
| `SenderEmail` | Share sender's email |
| `ExpiryDate` | Share expiration date string |

---

## Flow

1. User shares a file with `send_email: true`
2. Service renders the "sharing" HTML template
3. Email is sent synchronously via the configured provider
4. On failure the error is logged and `notificationStatus: "failed"` is returned (HTTP request is not blocked)

---

## Notable details

- SMTP uses **plain auth** — credentials are sent only after STARTTLS upgrade
- No connection pooling — every email opens a new connection
- No email queue or retry mechanism
- SES uses `context.TODO()` — no request context propagation
