// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("fetchInstalledBackends", () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    vi.resetModules();
    vi.restoreAllMocks();
    window.localStorage.clear();
    window.history.replaceState(
      null,
      "",
      "/api/v1/plugins/42/admin?token=test-token",
    );
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  function authHeaderAt(
    fetchMock: ReturnType<typeof vi.fn<typeof fetch>>,
    callIndex: number,
  ): string | null {
    const [, init] = fetchMock.mock.calls[callIndex] ?? [];
    return new Headers(init?.headers).get("Authorization");
  }

  it("filters ebook library sources and forwards the plugin token", async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            id: 11,
            plugin_id: "silo-plugin-bookwarehouse-ebook",
            enabled: true,
            capabilities: [
              {
                type: "ebook_backend.v1",
                id: "bookwarehouse",
                display_name: "Bookwarehouse",
                description: "Warehouse backend",
                metadata: {
                  ebook_roles: ["library_source", "download_provider"],
                },
              },
            ],
          },
          {
            id: 12,
            plugin_id: "silo-plugin-something-else",
            enabled: true,
            capabilities: [
              {
                type: "ebook_backend.v1",
                id: "downloads-only",
                display_name: "Downloads only",
                metadata: {
                  ebook_roles: ["download_provider"],
                },
              },
            ],
          },
        ]),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    globalThis.fetch = fetchMock;

    const { fetchInstalledBackends } = await import("./api");
    const backends = await fetchInstalledBackends();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/admin/plugins/installations",
      expect.objectContaining({
        credentials: "include",
      }),
    );
    expect(authHeaderAt(fetchMock, 0)).toBe("Bearer test-token");
    expect(backends).toEqual([
      expect.objectContaining({
        id: 11,
        plugin_id: "silo-plugin-bookwarehouse-ebook",
        display_name: "Bookwarehouse",
        ebook_roles: ["library_source", "download_provider"],
      }),
    ]);
  });

  it("refreshes and retries host backend discovery after a 401", async () => {
    window.localStorage.setItem("refresh_token", "refresh-token-1");

    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        new Response("<html>401</html>", {
          status: 401,
          headers: { "Content-Type": "text/html" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            access_token: "fresh-access-token",
            refresh_token: "refresh-token-2",
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify([
            {
              id: 21,
              plugin_id: "silo-plugin-local-ebooks",
              enabled: true,
              capabilities: [
                {
                  type: "ebook_backend.v1",
                  id: "local-ebooks",
                  display_name: "Local Ebooks",
                  metadata: {
                    ebook_roles: ["library_source"],
                  },
                },
              ],
            },
          ]),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      );
    globalThis.fetch = fetchMock;

    const { fetchInstalledBackends } = await import("./api");
    const backends = await fetchInstalledBackends();

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/v1/admin/plugins/installations",
      expect.objectContaining({
        credentials: "include",
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/v1/auth/refresh", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: "refresh-token-1" }),
      credentials: "include",
    });
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "/api/v1/admin/plugins/installations",
      expect.objectContaining({
        credentials: "include",
      }),
    );
    expect(authHeaderAt(fetchMock, 0)).toBe("Bearer test-token");
    expect(authHeaderAt(fetchMock, 2)).toBe("Bearer fresh-access-token");
    expect(window.localStorage.getItem("refresh_token")).toBe("refresh-token-2");
    expect(backends).toEqual([
      expect.objectContaining({
        id: 21,
        plugin_id: "silo-plugin-local-ebooks",
        ebook_roles: ["library_source"],
      }),
    ]);
  });
});
