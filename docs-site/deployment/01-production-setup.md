# Production setup

## Prerequisites

FluxSend can be deployed on any Linux VPS, provided all [needed tools](../quickstart/01-prerequisites.md) and [cloud components](../config-reference/01-env-vars.md) are set up.

Most frictionless deployment path right now is setting up the Docker Compose stack.

## Bare minimum

To get minimum functionality out of the service, it is recommended to set up:

- [Cloud storage (GCS or S3)](../config-reference/02-storage.md)
- <b>At least one</b> [OAuth2 provider](../config-reference/03-auth.md)
- An [SMTP sender](../config-reference/06-smtp.md) to support mail delivery

## Full setup

An optional service you can try deploying is Amazon Cloudfront which requires an S3 provider to work. Refer to [CDN section](../config-reference/05-cdn.md) to get more details on setting up the integration.

## Reverse Proxy

To expose the service and/or configure TLS termination for a custom domain, using a reverse proxy of your choice is highly encouraged.

Reverse proxy stacks to consider:

- Nginx + Certbot (popular and reliable, semi-automatic certs provisioning)
- HAProxy + Certbot (performant, semi-automatic certs)
- Traefik (well-documented & reliable - with automatic certs)
- Caddy (modern, automatic certs)
- Kubernetes ingress controllers (complicated setup, most flexible, cloud-native approach)

## Deployment options

Follow one of the provided setup guides to deploy FluxSend on your environment.

You can deploy the app three main ways:

- [Standalone (systemd services)](./02-standalone.md)
- [Docker/Docker compose](./03-docker.md)
- [Kubernetes](./04-kubernetes.md)