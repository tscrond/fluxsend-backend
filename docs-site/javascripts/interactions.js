/* ==========================================================================
   FluxSend Docs — interaction layer
   --------------------------------------------------------------------------
   Small, dependency-free enhancements that make the docs feel like the app:
     · keeps the browser theme colour in sync with the Material palette
     · keeps embedded API docs in sync with the selected palette
     · staggered reveal-on-scroll for content blocks
   Everything degrades gracefully and respects prefers-reduced-motion.
   ========================================================================== */
(function () {
  'use strict';

  var reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  /* --- browser chrome ------------------------------------------------------ */

  function syncThemeColor() {
    var meta = document.querySelector('meta[name="theme-color"]');
    if (!meta) return;
    var scheme =
      (document.body && document.body.getAttribute('data-md-color-scheme')) ||
      document.documentElement.getAttribute('data-md-color-scheme');
    var dark = scheme !== 'default';
    meta.setAttribute('content', dark ? '#0d1117' : '#f6f8fa');
  }

  /* --- embedded API reference --------------------------------------------- */

  function syncSwaggerTheme() {
    var scheme =
      (document.body && document.body.getAttribute('data-md-color-scheme')) ||
      document.documentElement.getAttribute('data-md-color-scheme');
    var theme = scheme === 'default' ? 'light' : 'dark';

    document.querySelectorAll('.swagger-frame iframe').forEach(function (iframe) {
      var applyTheme = function () {
        try {
          iframe.contentDocument.documentElement.setAttribute('data-theme', theme);
        } catch (error) {}
      };

      if (iframe.dataset.fsThemeBound !== 'true') {
        iframe.addEventListener('load', applyTheme);
        iframe.dataset.fsThemeBound = 'true';
      }
      applyTheme();
    });
  }

  /* --- reveal on scroll ---------------------------------------------------- */

  var REVEAL_SELECTOR = [
    '.md-content__inner > h2',
    '.md-content__inner > h3',
    '.md-content__inner > .admonition',
    '.md-content__inner > details',
    '.md-content__inner > table',
    '.md-content__inner > .highlight',
    '.md-content__inner > .tabbed-set',
    '.md-content__inner > .fs-cards',
    '.md-content__inner > .fs-map',
    '.md-content__inner > blockquote',
  ].join(',');

  function bindReveal(root) {
    var nodes = root.querySelectorAll(REVEAL_SELECTOR);
    if (!nodes.length) return;

    if (reduceMotion || !('IntersectionObserver' in window)) {
      nodes.forEach(function (node) {
        node.classList.add('fs-in');
      });
      return;
    }

    var observer = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (!entry.isIntersecting) return;
          entry.target.classList.add('fs-in');
          observer.unobserve(entry.target);
        });
      },
      { rootMargin: '0px 0px -6% 0px', threshold: 0.05 }
    );

    var index = 0;
    nodes.forEach(function (node) {
      // Leave already-visible blocks alone so the first paint stays stable.
      if (node.getBoundingClientRect().top < window.innerHeight * 0.85) return;
      node.classList.add('fs-reveal');
      node.style.setProperty('--fs-delay', Math.min(index % 5, 4) * 50 + 'ms');
      observer.observe(node);
      index += 1;
    });
  }

  /* --- wiring -------------------------------------------------------------- */

  function init() {
    syncThemeColor();
    syncSwaggerTheme();
    bindReveal(document);
  }

  var documentBundle =
    typeof window.document$ !== 'undefined' && window.document$
      ? window.document$
      : null;

  if (documentBundle && typeof documentBundle.subscribe === 'function') {
    documentBundle.subscribe(init);
  } else if (document.readyState !== 'loading') {
    init();
  } else {
    document.addEventListener('DOMContentLoaded', init);
  }

  // Keep the browser chrome in sync when the palette is toggled (or when the
  // system preference flips while using the auto palette).
  var themeObserver = new MutationObserver(function () {
    syncThemeColor();
    syncSwaggerTheme();
  });
  themeObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['data-md-color-scheme'],
  });
  if (document.body) {
    themeObserver.observe(document.body, {
      attributes: true,
      attributeFilter: ['data-md-color-scheme'],
    });
  }

  if (window.matchMedia) {
    window
      .matchMedia('(prefers-color-scheme: dark)')
      .addEventListener('change', syncThemeColor);
  }
})();
