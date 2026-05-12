// API base is /api/v1/plugins/{installationId}, detected at runtime since
// the installation ID is assigned at install time (not known at build time).
function apiBase(): string {
  const m = window.location.pathname.match(/^(\/api\/v1\/plugins\/\d+)/);
  return m ? m[1] : '';
}

// Continuum's plugin proxy authenticates each request via a Bearer token
// (Authorization header) or ?token= query param. The SPA receives the token
// on its initial load via URL ?token= (set by the sidebar link click). We
// capture it once into memory for use on all subsequent fetches.
let cachedToken: string | null = null;
let cachedTheme: string | null = null;
(function captureFromURL() {
  const params = new URLSearchParams(window.location.search);
  const t = params.get('token');
  if (t) {
    cachedToken = t;
    params.delete('token');
  }
  const th = params.get('theme') ?? sessionStorage.getItem('continuum-theme');
  if (th) {
    cachedTheme = th;
    try { sessionStorage.setItem('continuum-theme', th); } catch { /* private mode */ }
  }
  if (t) {
    const clean = window.location.pathname + (params.toString() ? '?' + params.toString() : '') + window.location.hash;
    window.history.replaceState(null, '', clean);
  }
})();

export function getCachedToken(): string | null {
  return cachedToken;
}

export function getCachedTheme(): string | null {
  return cachedTheme;
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (cachedToken) headers['Authorization'] = `Bearer ${cachedToken}`;
  const res = await fetch(`${apiBase()}${path}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'include',
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: { message: res.statusText } }));
    throw new Error(err.error?.message ?? 'Request failed');
  }
  if (res.status === 204) return undefined as T;
  return await res.json();
}

export const api = {
  get: <T,>(p: string) => call<T>('GET', p),
  post: <T,>(p: string, body?: unknown) => call<T>('POST', p, body),
  patch: <T,>(p: string, body?: unknown) => call<T>('PATCH', p, body),
  put: <T,>(p: string, body?: unknown) => call<T>('PUT', p, body),
  delete: <T,>(p: string) => call<T>('DELETE', p),
  fetchRaw: async (path: string) => {
    const headers: Record<string, string> = {};
    if (cachedToken) headers['Authorization'] = `Bearer ${cachedToken}`;
    return fetch(`${apiBase()}${path}`, { headers, credentials: 'include' });
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

export type Request = {
  id: string;
  user_id: string;
  title: string;
  authors?: string[];
  isbn?: string;
  source_id?: string;
  format_pref?: string;
  status: string;
  external_id?: string;
  target_plugin_id: string;
  auto_monitor?: boolean;
  fulfilled_book_id?: string;
  denied_reason?: string;
  failure_reason?: string;
  created_at?: string;
  updated_at?: string;
};

export type Collection = {
  id: string;
  user_id: string;
  name: string;
  color?: string;
  is_public?: boolean;
  is_pinned?: boolean;
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
  created_at?: string;
};

// -- Catalog ---------------------------------------------------------------

export const listCatalog = (cursor = '', sort = 'added', order = 'desc', limit = 50) =>
  api.get<PageEnvelope<EbookSummary>>(
    `/api/v1/ebooks?cursor=${encodeURIComponent(cursor)}&sort=${sort}&order=${order}&limit=${limit}`,
  );

export const getBook = (id: string) =>
  api.get<EbookDetail>(`/api/v1/ebooks/${encodeURIComponent(id)}`);

export const searchCatalog = (q: string) =>
  api.get<PageEnvelope<EbookSummary>>(`/api/v1/ebooks/search?q=${encodeURIComponent(q)}`);

// -- Library / progress / annotations --------------------------------------

export const fetchLibrary = (status = '') =>
  api.get<{ items: UserData[] }>(`/api/v1/me/library?status=${encodeURIComponent(status)}`);

export const updateProgress = (bookID: string, body: Partial<UserData>) =>
  api.post(`/api/v1/me/books/${encodeURIComponent(bookID)}/progress`, body);

export const updateBookMeta = (bookID: string, body: Partial<UserData>) =>
  api.patch(`/api/v1/me/books/${encodeURIComponent(bookID)}`, body);

export const listAnnotations = (bookID: string) =>
  api.get<{ items: Annotation[] }>(`/api/v1/me/books/${encodeURIComponent(bookID)}/annotations`);

export const createAnnotation = (bookID: string, body: Partial<Annotation>) =>
  api.post<Annotation>(`/api/v1/me/books/${encodeURIComponent(bookID)}/annotations`, body);

export const updateAnnotation = (annID: string, body: Partial<Annotation>) =>
  api.patch(`/api/v1/me/annotations/${encodeURIComponent(annID)}`, body);

export const deleteAnnotation = (annID: string) =>
  api.delete(`/api/v1/me/annotations/${encodeURIComponent(annID)}`);

// -- Requests --------------------------------------------------------------

export const listMyRequests = () => api.get<{ items: Request[] }>(`/api/v1/me/requests`);

export const createRequest = (body: Partial<Request>) =>
  api.post<Request>(`/api/v1/me/requests`, body);

export const cancelRequest = (id: string) =>
  api.delete(`/api/v1/me/requests/${encodeURIComponent(id)}`);

// -- Collections -----------------------------------------------------------

export const listMyCollections = () => api.get<{ items: Collection[] }>(`/api/v1/me/collections`);

export const createCollection = (body: Partial<Collection>) =>
  api.post<Collection>(`/api/v1/me/collections`, body);

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
  api.post<{ id: string; label: string; jti_shown_once: string }>(`/api/v1/me/opds-tokens`, {
    label,
  });

export const revokeOPDSToken = (id: string) =>
  api.delete(`/api/v1/me/opds-tokens/${encodeURIComponent(id)}`);

// -- Kosync ----------------------------------------------------------------

export type KosyncStatus = { registered: boolean; kosync_username?: string };

export const getKosyncStatus = () => api.get<KosyncStatus>(`/api/v1/me/kosync`);

export const registerKosync = (username: string, password: string) =>
  api.post(`/api/v1/me/kosync/register`, { username, password });

export const deleteKosync = () => api.delete(`/api/v1/me/kosync`);

// -- Kindle / Kobo ---------------------------------------------------------

export const sendToKindle = (bookID: string, format: string, toAddress: string) =>
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

export const adminListRequests = (status = '') =>
  api.get<{ items: Request[] }>(
    `/api/v1/admin/requests${status ? `?status=${encodeURIComponent(status)}` : ''}`,
  );

export const adminPatchRequest = (
  id: string,
  body: { action: string; denied_reason?: string; fulfilled_book_id?: string },
) => api.patch(`/api/v1/admin/requests/${encodeURIComponent(id)}`, body);

export type BackendConfig = {
  target_backend_plugin_id: string;
  auto_approve_requests: boolean;
  default_streaming_mode: string;
  cache_dir: string;
  cache_max_size_gb: number;
  cache_download_concurrency: number;
  opds_realm: string;
  kepubify_path: string;
};

export const adminGetBackend = () => api.get<BackendConfig>(`/api/v1/admin/backend`);

export const adminPatchBackend = (body: Partial<BackendConfig>) =>
  api.patch(`/api/v1/admin/backend`, body);

export const adminCacheStats = () =>
  api.get<{ bytes_used: number; bytes_max: number }>(`/api/v1/admin/cache`);

export const adminCacheLargest = () =>
  api.get<{
    items: { id: string; book_id: string; format: string; bytes_on_disk: number }[];
  }>(`/api/v1/admin/cache/largest`);

// -- Installed-backends discovery (direct host call) -----------------------

export type InstalledBackend = {
  id: number;
  plugin_id: string;
  display_name: string;
  enabled: boolean;
};

export async function fetchInstalledBackends(): Promise<InstalledBackend[]> {
  const headers: Record<string, string> = {};
  if (cachedToken) headers['Authorization'] = `Bearer ${cachedToken}`;
  const res = await fetch('/api/v1/admin/plugins/installations', {
    headers,
    credentials: 'include',
  });
  if (!res.ok) return [];
  const body = await res.json();
  const installations = Array.isArray(body) ? body : body.installations || [];
  return installations
    .filter(
      (i: { enabled: boolean; capabilities?: { type: string }[] }) =>
        i.enabled && (i.capabilities ?? []).some((c) => c.type === 'ebook_backend.v1'),
    )
    .map((i: { id: number; plugin_id: string; display_name?: string; enabled: boolean }) => ({
      id: i.id,
      plugin_id: i.plugin_id,
      display_name: i.display_name || i.plugin_id,
      enabled: i.enabled,
    }));
}
