// API base is /api/v1/plugins/{installationId}, detected at runtime since
// the installation ID is assigned at install time (not known at build time).
function apiBase(): string {
  const m = window.location.pathname.match(/^(\/api\/v1\/plugins\/\d+)/);
  return m ? m[1] : "";
}

// Continuum's plugin proxy authenticates each request via a Bearer token
// (Authorization header) or ?token= query param. The SPA receives the token
// on its initial load via URL ?token= (set by the sidebar link click). We
// capture it once into memory for use on all subsequent fetches.
let cachedToken: string | null = null;
let cachedTheme: string | null = null;
(function captureFromURL() {
  const params = new URLSearchParams(window.location.search);
  const t = params.get("token");
  if (t) {
    cachedToken = t;
    params.delete("token");
  }
  const th = params.get("theme") ?? sessionStorage.getItem("continuum-theme");
  if (th) {
    cachedTheme = th;
    try {
      sessionStorage.setItem("continuum-theme", th);
    } catch {
      /* private mode */
    }
  }
  if (t) {
    const clean =
      window.location.pathname +
      (params.toString() ? "?" + params.toString() : "") +
      window.location.hash;
    window.history.replaceState(null, "", clean);
  }
})();

export function getCachedToken(): string | null {
  return cachedToken;
}

export function getCachedTheme(): string | null {
  return cachedTheme;
}

async function call<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (cachedToken) headers["Authorization"] = `Bearer ${cachedToken}`;
  const res = await fetch(`${apiBase()}${path}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: "include",
  });
  if (!res.ok) {
    const err = await res
      .json()
      .catch(() => ({ error: { message: res.statusText } }));
    throw new Error(err.error?.message ?? "Request failed");
  }
  if (res.status === 204) return undefined as T;
  return await res.json();
}

export const api = {
  get: <T>(p: string) => call<T>("GET", p),
  post: <T>(p: string, body?: unknown) => call<T>("POST", p, body),
  patch: <T>(p: string, body?: unknown) => call<T>("PATCH", p, body),
  put: <T>(p: string, body?: unknown) => call<T>("PUT", p, body),
  delete: <T>(p: string) => call<T>("DELETE", p),
  fetchRaw: async (path: string) => {
    const headers: Record<string, string> = {};
    if (cachedToken) headers["Authorization"] = `Bearer ${cachedToken}`;
    return fetch(`${apiBase()}${path}`, { headers, credentials: "include" });
  },
};

export function mountPath(): string {
  return apiBase();
}

// -- Types matching the Go REST API -----------------------------------------

export type Identity = {
  user_id: string;
  username: string;
  email: string;
  is_admin: boolean;
};

export type EbookSummary = {
  id: string;
  library_id?: number;
  library_name?: string;
  media_type?: string;
  title: string;
  authors?: string[];
  series?: string;
  series_index?: number;
  year?: number;
  language?: string;
  cover_url?: string;
  has_cover: boolean;
  rating?: number;
  formats: string[];
};

export type LibraryInfo = {
  id: number;
  name: string;
  path?: string;
  media_type: string;
  backend_plugin_id?: string;
  backend_library_id?: number;
  enabled: boolean;
  sort_order?: number;
  last_scanned_at?: string;
};

export type EbookFile = {
  format: string;
  size_bytes: number;
  mime_type: string;
  url?: string;
};

export type EbookDetail = EbookSummary & {
  description?: string;
  isbn?: string;
  publisher?: string;
  genres?: string[];
  tags?: string[];
  files: EbookFile[];
  upstream_id?: string;
};

export type PageEnvelope<T> = {
  items: T[];
  next_cursor?: string;
  total?: number;
};

export type UserData = {
  user_id: string;
  book_id: string;
  last_cfi?: string;
  current_page?: number;
  read_progress?: number;
  is_finished?: boolean;
  is_favorite?: boolean;
  rating?: number;
  notes?: string;
  last_read_at?: string;
};

export type ReaderConfigEnvelope = {
  book_id: string;
  config: Record<string, unknown>;
  updated_at?: string;
};

export type ExternalReaderProgress = {
  source: "kosync" | string;
  document: string;
  progress: string;
  percentage?: number;
  device?: string;
  device_id?: string;
  timestamp?: string;
  canResume?: boolean;
  location?: string;
};

export type Request = {
  id: string;
  user_id: string;
  title: string;
  authors?: string[];
  isbn?: string;
  source_id?: string;
  format_pref?: string;
  media_type?: string;
  status: string;
  external_id?: string;
  target_plugin_id: string;
  auto_monitor?: boolean;
  fulfilled_book_id?: string;
  denied_reason?: string;
  failure_reason?: string;
  created_at?: string;
  updated_at?: string;
  fulfilled_at?: string | null;
};

export type Collection = {
  id: string;
  user_id: string;
  name: string;
  color?: string;
  is_public?: boolean;
  is_pinned?: boolean;
  cover_book_id?: string;
};

export type Annotation = {
  id: string;
  user_id: string;
  book_id: string;
  cfi_range: string;
  kind: string;
  color?: string;
  selected_text?: string;
  note_text?: string;
  readest_type?: "bookmark" | "annotation" | "excerpt" | string;
  xpointer0?: string;
  xpointer1?: string;
  page?: number;
  style?: "highlight" | "underline" | "squiggly" | string;
  metadata_json?: Record<string, unknown>;
  deleted_at?: string;
  created_at?: string;
  updated_at?: string;
};

// -- Catalog ---------------------------------------------------------------

export type CatalogFilters = {
  library_id?: number;
  author?: string;
  series?: string;
  genre?: string; // upstream slug (NOT the row id), see backend BrowseGenres remap
  tag?: string;
};

export const listCatalog = (
  cursor = "",
  sort = "added",
  order = "desc",
  limit = 50,
  filters: CatalogFilters = {},
) => {
  const params = new URLSearchParams({
    cursor,
    sort,
    order,
    limit: String(limit),
  });
  if (filters.author) params.set("author", filters.author);
  if (filters.library_id) params.set("library_id", String(filters.library_id));
  if (filters.series) params.set("series", filters.series);
  if (filters.genre) params.set("genre", filters.genre);
  if (filters.tag) params.set("tag", filters.tag);
  return api.get<PageEnvelope<EbookSummary>>(
    `/api/v1/ebooks?${params.toString()}`,
  );
};

export const listLibraries = () =>
  api.get<{ items: LibraryInfo[] }>(`/api/v1/libraries`);

export const getBook = (id: string) =>
  api.get<EbookDetail>(`/api/v1/ebooks/${encodeURIComponent(id)}`);

export const searchCatalog = (q: string, libraryID?: number) => {
  const params = new URLSearchParams({ q });
  if (libraryID) params.set("library_id", String(libraryID));
  return api.get<PageEnvelope<EbookSummary>>(
    `/api/v1/ebooks/search?${params.toString()}`,
  );
};

// -- Browse facets ---------------------------------------------------------

export type FacetItem = {
  id: string;
  name: string;
  count?: number;
};

export const browseAuthors = (cursor = "", limit = 50, libraryID?: number) => {
  const params = new URLSearchParams({ cursor, limit: String(limit) });
  if (libraryID) params.set("library_id", String(libraryID));
  return api.get<PageEnvelope<FacetItem>>(
    `/api/v1/browse/authors?${params.toString()}`,
  );
};

export const browseSeries = (cursor = "", limit = 50, libraryID?: number) => {
  const params = new URLSearchParams({ cursor, limit: String(limit) });
  if (libraryID) params.set("library_id", String(libraryID));
  return api.get<PageEnvelope<FacetItem>>(
    `/api/v1/browse/series?${params.toString()}`,
  );
};

export const browseGenres = (cursor = "", limit = 50, libraryID?: number) => {
  const params = new URLSearchParams({ cursor, limit: String(limit) });
  if (libraryID) params.set("library_id", String(libraryID));
  return api.get<PageEnvelope<FacetItem>>(
    `/api/v1/browse/genres?${params.toString()}`,
  );
};

// -- Progress / annotations -----------------------------------------------

export const listRecentProgress = () =>
  api.get<{ items: UserData[] }>(`/api/v1/me/progress`);

export const getBookUserData = (bookID: string) =>
  api.get<UserData>(`/api/v1/me/books/${encodeURIComponent(bookID)}`);

export const updateProgress = (bookID: string, body: Partial<UserData>) =>
  api.post(`/api/v1/me/books/${encodeURIComponent(bookID)}/progress`, body);

export const updateBookMeta = (bookID: string, body: Partial<UserData>) =>
  api.patch(`/api/v1/me/books/${encodeURIComponent(bookID)}`, body);

export const getReaderConfig = (bookID: string) =>
  api.get<ReaderConfigEnvelope>(
    `/api/v1/me/books/${encodeURIComponent(bookID)}/reader-config`,
  );

export const putReaderConfig = (
  bookID: string,
  config: Record<string, unknown>,
) =>
  api.put<ReaderConfigEnvelope>(
    `/api/v1/me/books/${encodeURIComponent(bookID)}/reader-config`,
    { config },
  );

export const linkKosyncBook = (
  bookID: string,
  body: { document: string; format?: string },
) =>
  api.post<{ book_id: string; document: string; format: string }>(
    `/api/v1/me/books/${encodeURIComponent(bookID)}/kosync-link`,
    body,
  );

export const listAnnotations = (bookID: string) =>
  api.get<{ items: Annotation[] }>(
    `/api/v1/me/books/${encodeURIComponent(bookID)}/annotations`,
  );

export const createAnnotation = (bookID: string, body: Partial<Annotation>) =>
  api.post<Annotation>(
    `/api/v1/me/books/${encodeURIComponent(bookID)}/annotations`,
    body,
  );

export const updateAnnotation = (annID: string, body: Partial<Annotation>) =>
  api.patch(`/api/v1/me/annotations/${encodeURIComponent(annID)}`, body);

export const deleteAnnotation = (annID: string) =>
  api.delete(`/api/v1/me/annotations/${encodeURIComponent(annID)}`);

// -- Requests --------------------------------------------------------------

export const listMyRequests = () =>
  api.get<{ items: Request[] }>(`/api/v1/me/requests`);

export const getMyRequest = (id: string) =>
  api.get<Request>(`/api/v1/me/requests/${encodeURIComponent(id)}`);

export const createRequest = (body: Partial<Request>) =>
  api.post<Request>(`/api/v1/me/requests`, body);

export type RequestRoutingPreview = {
  media_type: string;
  target_plugin_id: string;
  format_pref?: string;
  auto_monitor?: boolean;
  source: "rule" | "default";
};

export const previewRequestRouting = (mediaType: string) =>
  api.get<RequestRoutingPreview>(
    `/api/v1/request-routing/preview?media_type=${encodeURIComponent(mediaType)}`,
  );

export const cancelRequest = (id: string) =>
  api.delete(`/api/v1/me/requests/${encodeURIComponent(id)}`);

// -- Collections -----------------------------------------------------------

export const listMyCollections = () =>
  api.get<{ items: Collection[] }>(`/api/v1/me/collections`);

export const createCollection = (body: Partial<Collection>) =>
  api.post<Collection>(`/api/v1/me/collections`, body);

export const updateCollection = (id: string, body: Partial<Collection>) =>
  api.patch<Collection>(
    `/api/v1/me/collections/${encodeURIComponent(id)}`,
    body,
  );

export const deleteCollection = (id: string) =>
  api.delete(`/api/v1/me/collections/${encodeURIComponent(id)}`);

export const listCollectionItems = (id: string) =>
  api.get<{ items: { book_id: string; position: number }[] }>(
    `/api/v1/me/collections/${encodeURIComponent(id)}/items`,
  );

export const addCollectionItem = (id: string, bookID: string, position = 0) =>
  api.post(`/api/v1/me/collections/${encodeURIComponent(id)}/items`, {
    book_id: bookID,
    position,
  });

export const removeCollectionItem = (id: string, bookID: string) =>
  api.delete(
    `/api/v1/me/collections/${encodeURIComponent(id)}/items/${encodeURIComponent(bookID)}`,
  );

// -- OPDS tokens -----------------------------------------------------------

export type OPDSToken = {
  id: string;
  label?: string;
  last_used_at?: string;
  created_at?: string;
  revoked?: boolean;
};

export const listOPDSTokens = () =>
  api.get<{ items: OPDSToken[] }>(`/api/v1/me/opds-tokens`);

export const createOPDSToken = (label: string) =>
  api.post<{ id: string; label: string; jti_shown_once: string }>(
    `/api/v1/me/opds-tokens`,
    {
      label,
    },
  );

export const revokeOPDSToken = (id: string) =>
  api.delete(`/api/v1/me/opds-tokens/${encodeURIComponent(id)}`);

// -- Kosync ----------------------------------------------------------------

export type KosyncStatus = { registered: boolean; kosync_username?: string };

export const getKosyncStatus = () => api.get<KosyncStatus>(`/api/v1/me/kosync`);

export const registerKosync = (username: string, password: string) =>
  api.post(`/api/v1/me/kosync/register`, { username, password });

export const deleteKosync = () => api.delete(`/api/v1/me/kosync`);

// -- Kindle / Kobo ---------------------------------------------------------

export const sendToKindle = (
  bookID: string,
  format: string,
  toAddress: string,
) =>
  api.post<{ id: string; status: string }>(
    `/api/v1/me/books/${encodeURIComponent(bookID)}/send-to-kindle`,
    { format, to_address: toAddress },
  );

export type KoboTransferResponse = {
  transfer_code: string;
  transfer_url: string;
  expires_at: string;
};

export const sendToKobo = (bookID: string) =>
  api.post<KoboTransferResponse>(
    `/api/v1/me/books/${encodeURIComponent(bookID)}/send-to-kobo`,
    {},
  );

// -- Identity --------------------------------------------------------------

export const fetchIdentity = () => api.get<Identity>(`/api/v1/me`);

// -- Admin -----------------------------------------------------------------

export const adminListRequests = (status = "") =>
  api.get<{ items: Request[] }>(
    `/api/v1/admin/requests${status ? `?status=${encodeURIComponent(status)}` : ""}`,
  );

export const adminPatchRequest = (
  id: string,
  body: { action: string; denied_reason?: string; fulfilled_book_id?: string },
) => api.patch(`/api/v1/admin/requests/${encodeURIComponent(id)}`, body);

export const adminBulkRequests = (body: {
  ids: string[];
  action: string;
  denied_reason?: string;
}) => api.post<{ updated: number }>(`/api/v1/admin/requests/bulk`, body);

export type ProviderHealth = {
  ok: boolean;
  message: string;
  formats?: string[];
  features?: string[];
  max_concurrent_downloads?: number;
  supports_range_requests?: boolean;
};

export type ProviderTestSearch = {
  ok: boolean;
  message: string;
  items: EbookSummary[];
};

export type RequestRoutingRule = {
  id: number;
  media_type: string;
  target_plugin_id: string;
  format_pref?: string;
  auto_monitor: boolean;
  enabled: boolean;
  sort_order?: number;
};

export const adminProviderHealth = (id: string) =>
  api.get<ProviderHealth>(
    `/api/v1/admin/providers/${encodeURIComponent(id)}/health`,
  );

export const adminProviderTestSearch = (id: string, q: string) =>
  api.get<ProviderTestSearch>(
    `/api/v1/admin/providers/${encodeURIComponent(id)}/test-search?q=${encodeURIComponent(q)}`,
  );

export const adminListRoutingRules = () =>
  api.get<{ items: RequestRoutingRule[] }>(`/api/v1/admin/routing-rules`);

export const adminReplaceRoutingRules = (items: RequestRoutingRule[]) =>
  api.put(`/api/v1/admin/routing-rules`, { items });

export type BackendConfig = {
  target_backend_plugin_id: string;
  auto_approve_requests: boolean;
  default_streaming_mode: string;
  cache_dir: string;
  cache_max_size_gb: number;
  cache_download_concurrency: number;
  opds_realm: string;
  kepubify_path: string;
  libraries?: LibraryInfo[];
};

export const adminGetBackend = () =>
  api.get<BackendConfig>(`/api/v1/admin/backend`);

export const adminPatchBackend = (body: Partial<BackendConfig>) =>
  api.patch(`/api/v1/admin/backend`, body);

export const adminListLibraries = () =>
  api.get<{ items: LibraryInfo[] }>(`/api/v1/admin/libraries`);

export const adminReplaceLibraries = (items: LibraryInfo[]) =>
  api.put(`/api/v1/admin/libraries`, { items });

export const adminListBackendLibraries = (backendPluginID: string) =>
  api.get<{ items: LibraryInfo[] }>(
    `/api/v1/admin/backend-libraries?backend_plugin_id=${encodeURIComponent(backendPluginID)}`,
  );

export const adminSyncLibraries = (backendPluginID: string) =>
  api.post<{ created: number; updated: number; pruned: number; kept: number }>(
    `/api/v1/admin/libraries/sync?backend_plugin_id=${encodeURIComponent(backendPluginID)}`,
  );

export const adminCacheStats = () =>
  api.get<{ bytes_used: number; bytes_max: number }>(`/api/v1/admin/cache`);

export const adminCacheLargest = () =>
  api.get<{
    items: {
      id: string;
      book_id: string;
      cache_key?: string;
      format: string;
      mime_type?: string;
      status?: string;
      error_message?: string;
      relative_path?: string;
      content_length?: number;
      bytes_on_disk: number;
      last_accessed_at?: string;
      created_at?: string;
    }[];
  }>(`/api/v1/admin/cache/largest`);

export type AdminKoboSession = {
  id: string;
  user_id: string;
  book_id: string;
  format: string;
  status: string;
  source_path?: string;
  created_at?: string;
  expires_at?: string;
  completed_at?: string | null;
};

export const adminKoboSessions = () =>
  api.get<{ items: AdminKoboSession[] }>(`/api/v1/admin/kobo-sessions`);

export type AdminOPDSToken = {
  id: string;
  user_id: string;
  jti?: string;
  label?: string;
  last_used_at?: string;
  created_at?: string;
  revoked_at?: string | null;
};

export const adminOPDSTokens = () =>
  api.get<{ items: AdminOPDSToken[] }>(`/api/v1/admin/opds-tokens`);

export const adminRevokeOPDSToken = (id: string) =>
  api.delete(`/api/v1/admin/opds-tokens/${encodeURIComponent(id)}`);

export type AdminKosyncUser = {
  user_id: string;
  kosync_username: string;
  created_at?: string;
};

export const adminKosyncUsers = () =>
  api.get<{ items: AdminKosyncUser[] }>(`/api/v1/admin/kosync-users`);

export const adminDeleteKosyncUser = (username: string) =>
  api.delete(`/api/v1/admin/kosync-users/${encodeURIComponent(username)}`);

export type AdminKindleSend = {
  id: string;
  user_id: string;
  book_id: string;
  format: string;
  to_address: string;
  status: string;
  error_text?: string;
  sent_at?: string | null;
  created_at?: string;
};

export const adminKindleLog = () =>
  api.get<{ items: AdminKindleSend[] }>(`/api/v1/admin/kindle-log`);

// -- Installed-backends discovery (direct host call) -----------------------

export type InstalledBackend = {
  id: number;
  plugin_id: string;
  display_name: string;
  enabled: boolean;
  capabilities: InstalledCapability[];
  ebook_backend?: InstalledCapability;
  ebook_roles: string[];
  summary?: string;
};

export type InstalledCapability = {
  type: string;
  id: string;
  display_name?: string;
  description?: string;
  metadata?: Record<string, unknown>;
};

function ebookBackendCapability(
  capabilities: InstalledCapability[],
): InstalledCapability | undefined {
  return capabilities.find(
    (capability) => capability.type === "ebook_backend.v1",
  );
}

function ebookRoles(capability?: InstalledCapability): string[] {
  const roles = capability?.metadata?.ebook_roles;
  return Array.isArray(roles)
    ? roles.filter((role): role is string => typeof role === "string")
    : [];
}

function hasEbookRole(plugin: InstalledBackend, role: string): boolean {
  return plugin.ebook_roles.includes(role);
}

async function fetchInstalledEbookPlugins(): Promise<InstalledBackend[]> {
  const headers: Record<string, string> = {};
  if (cachedToken) headers["Authorization"] = `Bearer ${cachedToken}`;
  const res = await fetch("/api/v1/admin/plugins/installations", {
    headers,
    credentials: "include",
  });
  if (!res.ok) {
    // Throw instead of returning [] so React Query surfaces the failure.
    // Silently returning an empty list rendered a misleading "no library
    // sources — install one" state when the real problem was an auth/host
    // error, making backend setup undebuggable.
    const detail = await res.text().catch(() => "");
    throw new Error(
      `Could not load installed backends (HTTP ${res.status})${
        detail ? `: ${detail.slice(0, 200)}` : ""
      }`,
    );
  }
  const body = await res.json();
  const installations = Array.isArray(body) ? body : body.installations || [];
  return installations
    .filter((i: { enabled: boolean; capabilities?: InstalledCapability[] }) => {
      const capabilities = i.capabilities ?? [];
      return i.enabled && !!ebookBackendCapability(capabilities);
    })
    .map(
      (i: {
        id: number;
        plugin_id: string;
        display_name?: string;
        enabled: boolean;
        capabilities?: InstalledCapability[];
        metadata?: Record<string, unknown>;
      }) => {
        const capabilities = i.capabilities ?? [];
        const ebookBackend = ebookBackendCapability(capabilities);
        return {
          id: i.id,
          plugin_id: i.plugin_id,
          enabled: i.enabled,
          capabilities,
          ebook_backend: ebookBackend,
          ebook_roles: ebookRoles(ebookBackend),
          display_name:
            ebookBackend?.display_name ||
            i.display_name ||
            (typeof i.metadata?.display_name === "string"
              ? i.metadata.display_name
              : undefined) ||
            i.plugin_id,
          summary: ebookBackend?.description,
        };
      },
    );
}

export async function fetchInstalledBackends(): Promise<InstalledBackend[]> {
  const plugins = await fetchInstalledEbookPlugins();
  return plugins.filter((plugin) => hasEbookRole(plugin, "library_source"));
}

export async function fetchDownloadProviders(): Promise<InstalledBackend[]> {
  const plugins = await fetchInstalledEbookPlugins();
  return plugins.filter((plugin) => hasEbookRole(plugin, "download_provider"));
}
