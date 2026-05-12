import { useMutation } from '@tanstack/react-query';
import { useNavigate } from 'react-router';
import { useState } from 'react';
import { toast } from 'sonner';
import { createRequest } from '@/lib/api';
import { Button } from '@/components/ui/button';

export default function Submit() {
  const nav = useNavigate();
  const [title, setTitle] = useState('');
  const [authors, setAuthors] = useState('');
  const [isbn, setIsbn] = useState('');
  const [formatPref, setFormatPref] = useState('epub');
  const [autoMonitor, setAutoMonitor] = useState(false);

  const m = useMutation({
    mutationFn: () =>
      createRequest({
        title,
        authors: authors
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean),
        isbn,
        format_pref: formatPref,
        auto_monitor: autoMonitor,
      }),
    onSuccess: () => {
      toast.success('Request submitted');
      nav('/me/requests');
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
        <Field
          label="Authors"
          value={authors}
          onChange={setAuthors}
          help="Comma-separated"
        />
        <Field label="ISBN" value={isbn} onChange={setIsbn} />
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">Format</label>
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
            {m.isPending ? 'Submitting…' : 'Submit'}
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
      {help && <p className="mt-0.5 text-xs text-muted-foreground/70">{help}</p>}
    </div>
  );
}
