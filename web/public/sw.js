// Minimal service worker for installability. We intentionally do not cache
// API responses or book bytes — files are signed, authenticated, and can be
// large. Caching them would either serve stale auth tokens or eat disk.
//
// The SW exists so the browser will offer "Install app" and let the portal
// run in standalone mode (separate window, separate app entry). On every
// fetch we passthrough to the network. If we want offline shell caching
// later, gate it behind a separate VERSION constant + cache.put.

self.addEventListener("install", () => {
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener("fetch", () => {
  // Passthrough: let the browser handle the network request normally.
});
