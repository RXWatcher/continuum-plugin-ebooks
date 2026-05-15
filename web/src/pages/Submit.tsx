import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router";
import { useState } from "react";
import { toast } from "sonner";
import {
  createRequest,
  fetchDownloadProviders,
  type InstalledBackend,
  previewRequestRouting,
  searchCatalog,
} from "@/lib/api";
import { Button } from "@/components/ui/button";

function providerSummary(provider: InstalledBackend) {
  return provider.summary || "Use this provider for its configured ebook acquisition workflow.";
}

export default function Submit() {
  const nav = useNavigate();
  const providers = useQuery({
    queryKey: ["download-providers"],
    queryFn: fetchDownloadProviders,
  });
  const [title, setTitle] = useState("");
  const [authors, setAuthors] = useState("");
  const [isbn, setIsbn] = useState("");
  const [sourceID, setSourceID] = useState("");
  const [formatPref, setFormatPref] = useState("epub");
  const [mediaType, setMediaType] = useState("book");
  const [autoMonitor, setAutoMonitor] = useState(false);
  const [providerID, setProviderID] = useState("");
  const selectedProvider = (providers.data ?? []).find(
    (provider) => String(provider.id) === providerID,
  );
  const existing = useQuery({
    queryKey: ["request-existing-search", title],
    queryFn: () => searchCatalog(title),
    enabled: title.trim().length >= 3,
  });
  const routingPreview = useQuery({
    queryKey: ["request-routing-preview", mediaType],
    queryFn: () => previewRequestRouting(mediaType),
    enabled: !providerID,
  });
  const automaticProvider = (providers.data ?? []).find(
    (provider) => String(provider.id) === routingPreview.data?.target_plugin_id,
  );

  const m = useMutation({
    mutationFn: () =>
      createRequest({
        title,
        authors: authors
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
        isbn,
        source_id: sourceID,
        format_pref: formatPref,
        media_type: mediaType,
        auto_monitor: autoMonitor,
        target_plugin_id: providerID,
      }),
    onSuccess: () => {
      toast.success("Request submitted");
      nav("/me/requests");
    },
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <div className="max-w-xl">
      <h1 className="mb-4 text-xl font-semibold">Submit a request</h1>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          m.mutate();
        }}
        className="space-y-3"
      >
        <Field label="Title" required value={title} onChange={setTitle} />
        {title.trim().length >= 3 && (
          <div className="rounded-lg border border-border bg-card p-3">
            <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Existing matches
            </div>
            {existing.isLoading ? (
              <p className="mt-2 text-sm text-muted-foreground">
                Searching the library...
              </p>
            ) : (existing.data?.items ?? []).length > 0 ? (
              <div className="mt-2 space-y-2">
                {(existing.data?.items ?? []).slice(0, 3).map((book) => (
                  <Link
                    key={book.id}
                    to={`/${encodeURIComponent(book.id)}`}
                    className="block rounded-md bg-muted/40 px-3 py-2 text-sm hover:bg-accent"
                  >
                    <span className="block font-medium">{book.title}</span>
                    <span className="block text-xs text-muted-foreground">
                      {(book.authors ?? []).join(", ") || "Unknown author"}
                    </span>
                  </Link>
                ))}
              </div>
            ) : (
              <p className="mt-2 text-sm text-muted-foreground">
                No close library matches found.
              </p>
            )}
          </div>
        )}
        <Field
          label="Authors"
          value={authors}
          onChange={setAuthors}
          help="Comma-separated"
        />
        <Field label="ISBN" value={isbn} onChange={setIsbn} />
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">
            Media type
          </label>
          <select
            value={mediaType}
            onChange={(e) => setMediaType(e.target.value)}
            className="w-full rounded-md border border-border bg-background px-2 py-1.5 text-sm"
          >
            <option value="book">Book</option>
            <option value="comic">Comic</option>
            <option value="manga">Manga</option>
            <option value="magazine">Magazine</option>
          </select>
        </div>
        {selectedProvider?.plugin_id === "continuum.annas-archive-downloader" && (
          <Field
            label="Anna's Archive source ID"
            value={sourceID}
            onChange={setSourceID}
            help="Optional, but direct Anna's Archive requests work best with an exact MD5/source ID."
          />
        )}
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">
            Download provider
          </label>
          <p className="mb-2 text-xs leading-5 text-muted-foreground">
            Choose where this request should be sent after approval. Providers
            download or monitor requests; they are separate from the libraries
            you browse.
          </p>
          <div className="grid gap-2">
            {(providers.data ?? []).length === 0 ? (
              <div className="rounded-md border border-border px-3 py-2 text-sm text-muted-foreground">
                Default provider
              </div>
            ) : (
              <>
                <button
                  type="button"
                  onClick={() => setProviderID("")}
                  className={[
                    "rounded-md border px-3 py-2 text-left text-sm transition-colors",
                    providerID
                      ? "border-border bg-background hover:bg-accent"
                      : "border-primary bg-primary/10 text-foreground",
                  ].join(" ")}
                >
                  <span className="block font-medium">Automatic routing</span>
                  <span className="block text-xs text-muted-foreground">
                    {automaticProvider
                      ? `${routingPreview.data?.source === "rule" ? "Rule" : "Default"} routes ${mediaType} to ${automaticProvider.display_name}.`
                      : `Use the admin routing rule for ${mediaType}, then fall back to the global default provider.`}
                  </span>
                </button>
                {(providers.data ?? []).map((provider) => {
                  const selected = providerID === String(provider.id);
                  return (
                    <button
                      key={provider.id}
                      type="button"
                      onClick={() => setProviderID(String(provider.id))}
                      className={[
                        "rounded-md border px-3 py-2 text-left text-sm transition-colors",
                        selected
                          ? "border-primary bg-primary/10 text-foreground"
                          : "border-border bg-background hover:bg-accent",
                      ].join(" ")}
                    >
                      <span className="block font-medium">
                        {provider.display_name}
                      </span>
                      <span className="block text-xs text-muted-foreground">
                        {provider.plugin_id}
                      </span>
                      <span className="mt-1 block text-xs text-muted-foreground">
                        {providerSummary(provider)}
                      </span>
                    </button>
                  );
                })}
              </>
            )}
          </div>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">
            Format
          </label>
          <select
            value={formatPref}
            onChange={(e) => setFormatPref(e.target.value)}
            className="rounded-md border border-border bg-background px-2 py-1.5 text-sm"
          >
            <option value="epub">EPUB</option>
            <option value="pdf">PDF</option>
            <option value="mobi">MOBI</option>
            <option value="azw3">AZW3</option>
          </select>
        </div>
        <label className="inline-flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={autoMonitor}
            onChange={(e) => setAutoMonitor(e.target.checked)}
          />
          Auto-monitor (where supported)
        </label>
        <div className="pt-2">
          <Button type="submit" disabled={!title || m.isPending}>
            {m.isPending ? "Submitting…" : "Submit"}
          </Button>
        </div>
      </form>
    </div>
  );
}

function Field({
  label,
  required,
  value,
  onChange,
  help,
}: {
  label: string;
  required?: boolean;
  value: string;
  onChange: (v: string) => void;
  help?: string;
}) {
  return (
    <div>
      <label className="mb-1 block text-xs font-medium text-muted-foreground">
        {label}
        {required && <span className="text-destructive"> *</span>}
      </label>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        required={required}
        className="w-full rounded-md border border-border bg-background px-2 py-1.5 text-sm"
      />
      {help && (
        <p className="mt-0.5 text-xs text-muted-foreground/70">{help}</p>
      )}
    </div>
  );
}
