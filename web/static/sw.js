// Keel Service Worker — minimal, for PWA installability
// No aggressive caching: Keel is a local tool, if the binary isn't running nothing works.

const CACHE_NAME = 'keel-v1';
const PRECACHE = [
  '/static/style.css',
  '/static/app.js',
  '/static/htmx.min.js',
  '/static/alpine.min.js',
  '/static/lucide-sprite.svg',
  '/static/xterm.css',
  '/static/xterm.min.js',
  '/static/xterm-addon-fit.min.js',
  '/static/ansi_up.min.js'
];

self.addEventListener('install', (e) => {
  e.waitUntil(
    caches.open(CACHE_NAME)
      .then((cache) => cache.addAll(PRECACHE))
      .then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', (e) => {
  e.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k)))
    ).then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', (e) => {
  // Only cache static assets; let API/HTML requests go to network
  if (!e.request.url.includes('/static/')) return;

  e.respondWith(
    fetch(e.request)
      .then((res) => {
        const clone = res.clone();
        caches.open(CACHE_NAME).then((cache) => cache.put(e.request, clone));
        return res;
      })
      .catch(() => caches.match(e.request))
  );
});
