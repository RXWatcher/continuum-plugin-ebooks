import { useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router';
import { Search, X } from 'lucide-react';

export default function SearchBar() {
  const nav = useNavigate();
  const loc = useLocation();
  const [q, setQ] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (loc.pathname === '/search') {
      const params = new URLSearchParams(loc.search);
      setQ(params.get('q') ?? '');
    } else {
      setQ('');
    }
  }, [loc.pathname, loc.search]);

  function submit() {
    const trimmed = q.trim();
    if (!trimmed) return;
    nav(`/search?q=${encodeURIComponent(trimmed)}`);
  }

  return (
    <div className="relative w-full">
      <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
      <input
        ref={inputRef}
        type="search"
        value={q}
        onChange={(e) => setQ(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            submit();
          }
        }}
        placeholder="Search title, author, ISBN…"
        autoCapitalize="none"
        autoComplete="off"
        spellCheck={false}
        className="w-full rounded-md border border-border bg-background px-9 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring/50"
      />
      {q && (
        <button
          type="button"
          onClick={() => {
            setQ('');
            inputRef.current?.focus();
          }}
          className="absolute right-2 top-1/2 -translate-y-1/2 rounded-full p-1.5 transition-colors hover:bg-accent"
          aria-label="Clear search"
        >
          <X className="size-3.5 text-muted-foreground/70" />
        </button>
      )}
    </div>
  );
}
