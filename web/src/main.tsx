import { QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";
import App from "./App";
import "./index.css";
import { getCachedTheme } from "./lib/api";
import { queryClient } from "./lib/queryClient";

// Detect the basename at runtime: the SPA is mounted under
// /api/v1/plugins/{installationId}/ and we don't know the ID at build time.
// Strip everything from the first SPA-route path onward and use the rest as
// basename for react-router.
function detectBasename(): string {
  const m = window.location.pathname.match(/^(\/api\/v1\/plugins\/\d+)/);
  return m ? m[1] : "/";
}

// Apply continuum's theme to the plugin's <html> so semantic Tailwind classes
// (bg-primary, bg-card, etc.) inherit continuum's color palette. Sidebar
// link clicks pass ?theme=<continuum-active-theme>.
const theme = getCachedTheme();
if (theme) {
  document.documentElement.dataset.theme = theme;
}

// Register the service worker so the browser will offer to install the portal
// as a PWA. The SW lives at <mountPath>/sw.js with its scope rooted at
// <mountPath>/ — anything broader would conflict with other plugins served
// from the same continuum origin. Failure is silent (dev / non-secure
// contexts); the SPA still works, just no install prompt.
if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    const base = detectBasename();
    const scope = base.endsWith("/") ? base : base + "/";
    void navigator.serviceWorker.register(scope + "sw.js", { scope }).catch(() => {});
  });
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter basename={detectBasename()}>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </React.StrictMode>,
);
