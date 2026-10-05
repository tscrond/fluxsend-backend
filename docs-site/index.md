<div class="fs-cards">
  <a class="fs-card" href="introduction/01-what-is-fluxsend/">
    <span class="fs-card__icon">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>
    </span>
    <span class="fs-card__title">Introduction</span>
    <span class="fs-card__desc">What FluxSend is, the features it ships with and the problems it solves.</span>
    <span class="fs-card__more">Start here
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M13 6l6 6-6 6"/></svg>
    </span>
  </a>
  <a class="fs-card" href="quickstart/01-prerequisites/">
    <span class="fs-card__icon">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/></svg>
    </span>
    <span class="fs-card__title">Quick start</span>
    <span class="fs-card__desc">Install the toolchain and get a local FluxSend stack running in minutes.</span>
    <span class="fs-card__more">Run it locally
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M13 6l6 6-6 6"/></svg>
    </span>
  </a>
  <a class="fs-card" href="config-reference/00-config-loading/">
    <span class="fs-card__icon">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3"/><path d="M1 14h6M9 8h6M17 16h6"/></svg>
    </span>
    <span class="fs-card__title">Configuration</span>
    <span class="fs-card__desc">Every flag, environment variable and storage, auth, database and CDN option.</span>
    <span class="fs-card__more">Read the reference
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M13 6l6 6-6 6"/></svg>
    </span>
  </a>
  <a class="fs-card" href="deployment/00-production-checklist/">
    <span class="fs-card__icon">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18M3 15h18"/></svg>
    </span>
    <span class="fs-card__title">Deployment</span>
    <span class="fs-card__desc">Production checklist, Docker, Kubernetes, self-hosted MinIO and metrics.</span>
    <span class="fs-card__more">Ship it
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M13 6l6 6-6 6"/></svg>
    </span>
  </a>
  <a class="fs-card" href="api/">
    <span class="fs-card__icon">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m8 6-6 6 6 6M16 6l6 6-6 6"/></svg>
    </span>
    <span class="fs-card__title">API reference</span>
    <span class="fs-card__desc">An interactive OpenAPI explorer generated from the Go backend annotations.</span>
    <span class="fs-card__more">Explore the API
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M13 6l6 6-6 6"/></svg>
    </span>
  </a>
  <a class="fs-card" href="https://github.com/tscrond/fluxsend">
    <span class="fs-card__icon">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 3v18h18"/><path d="m7 15 3-4 3 3 5-7"/></svg>
    </span>
    <span class="fs-card__title">Open source</span>
    <span class="fs-card__desc">Browse the source, file an issue or contribute on GitHub.</span>
    <span class="fs-card__more">View on GitHub
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M13 6l6 6-6 6"/></svg>
    </span>
  </a>
</div>

## Browse the docs

<div class="fs-map">
{{ generate_toc(navigation) }}
</div>

!!! tip "Regenerating the API reference"
    The API pages are generated from the Go annotations. Run `make swagger` before previewing or publishing so the OpenAPI spec, endpoint reference and Swagger UI stay in sync with the backend.
