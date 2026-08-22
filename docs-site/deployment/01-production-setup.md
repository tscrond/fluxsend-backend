# Production setup

FluxSend can be deployed on any Linux host, container host, or small VPS. For the most self-host-friendly configuration, use PostgreSQL + MinIO + password auth without a cloud provider.

## Recommended minimal architecture

For a private deployment, the simplest production setup is:

- PostgreSQL for the app database
- MinIO for object storage
- password auth enabled for the UI
- SMTP for registration and password-reset emails
- a reverse proxy for TLS and domain routing

This is the most straightforward way to run FluxSend without AWS, GCS, or a managed storage backend.

## Storage and auth choices

The app supports these combinations:

- MinIO + password auth: best self-hosted option
- S3 + OAuth or password auth: best for AWS-native deployments
- GCS + OAuth or password auth: best for Google Cloud deployments

See [Storage](../config-reference/02-storage.md) and [Authentication](../config-reference/03-auth.md).

## Full setup

Optional CloudFront integration is still available for S3-backed deployments. See [CDN](../config-reference/05-cdn.md).

## Reverse proxy

To expose the service and terminate TLS, use a reverse proxy in front of the backend and frontend.

Recommended options:

- Nginx + Certbot
- Traefik
- Caddy
- HAProxy
- Kubernetes ingress

## Deployment options

Use one of the following deployment patterns:

- [Standalone (systemd)](./02-standalone.md)
- [Docker Compose](./03-docker.md)
- [Self-hosted MinIO](./05-self-hosted-minio.md)
- [Kubernetes](./04-kubernetes.md)
