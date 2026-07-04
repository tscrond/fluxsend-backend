# CDN

FluxSend supports serving downloads through **AWS CloudFront** signed URLs instead of direct storage presigned URLs.

---

## Requirements

- **Storage provider:** S3 only (`STORAGE_PROVIDER=s3`)
- **CloudFront distribution** with the S3 bucket configured as origin

---

## Environment variables

| Variable | Description | Required |
|---|---|---|
| `ENABLE_CLOUDFRONT_DOWNLOADS` | Set to `true` to enable CDN-signed URLs | No (default `false`) |
| `CLOUDFRONT_DOMAIN` | Distribution domain (e.g. `d123.cloudfront.net` or `cdn.fluxsend.win`) | Yes if enabled |
| `CLOUDFRONT_KEY_PAIR_ID` | CloudFront key pair ID for URL signing | Yes if enabled |
| `CLOUDFRONT_PRIVATE_KEY_PATH` | Path to the RSA private key PEM file | Yes if enabled |

---

## How it works

When `ENABLE_CLOUDFRONT_DOWNLOADS=true` and a user downloads a file, the share service signs a CloudFront URL instead of calling the storage provider's `GenerateSignedURL`.

**Signed URL format:**

```
https://{CLOUDFRONT_DOMAIN}/{userId}/{fileName}
```

Signing uses a **custom policy** with `DateLessThan` condition (no IP or referer restrictions). The CloudFront key pair is loaded from the PEM file specified in `CLOUDFRONT_PRIVATE_KEY_PATH` and must be an RSA private key in PKCS#8 format.

---

## CloudFront key pair

Generate a CloudFront key pair via the AWS Console (CloudFront > Key Management) or the AWS CLI. Download the private key `.pem` file and place it at the path referenced by `CLOUDFRONT_PRIVATE_KEY_PATH`.

---

## Domain format

`CLOUDFRONT_DOMAIN` accepts:
- A bare hostname: `cdn.fluxsend.win`
- A URL: `https://cdn.fluxsend.win` (path components are rejected)
