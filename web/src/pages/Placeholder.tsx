import { Construction } from 'lucide-react';

export default function Placeholder({ name }: { name: string }) {
  return (
    <div className="rounded-lg border border-dashed border-border p-12 text-center">
      <Construction className="mx-auto mb-3 size-8 text-muted-foreground" />
      <h2 className="mb-1 text-lg font-semibold">{name}</h2>
      <p className="text-sm text-muted-foreground">
        Coming soon — this view is derived from the catalog and will be added once the
        backend exposes the corresponding facet endpoints.
      </p>
    </div>
  );
}
