// The plugin SPA is served under /api/v1/plugins/{installationId}/admin/...
// {installationId} is not known at build time, so we derive it at runtime
// from window.location.pathname. Returns empty string on the dev server
// (no plugin proxy prefix).
export function extractMountPath(pathname: string): string {
  const m = pathname.match(/^(\/api\/v1\/plugins\/\d+)/);
  return m ? m[1] : '';
}

export function mountPath(): string {
  return extractMountPath(window.location.pathname);
}
