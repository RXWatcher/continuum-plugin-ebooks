import { useEffect, useRef, useState } from 'react';
import { useParams, Link } from 'react-router';
import ePub, { type Book, type Rendition } from 'epubjs';
import { ArrowLeft, ArrowRight, ChevronLeft } from 'lucide-react';
import { mountPath, updateProgress } from '@/lib/api';
import { Button } from '@/components/ui/button';

// Reader renders an EPUB inline using epub.js. The Go backend serves the
// raw bytes via /api/v1/me/books/{id}/file?format=epub (proxy or cache
// mode). Progress (CFI + percentage) is pushed to /me/books/{id}/progress.
export default function Reader() {
  const params = useParams();
  const id = params.id ?? '';
  const viewerRef = useRef<HTMLDivElement>(null);
  const renditionRef = useRef<Rendition | null>(null);
  const [book, setBook] = useState<Book | null>(null);
  const [pct, setPct] = useState(0);

  useEffect(() => {
    if (!id || !viewerRef.current) return;
    const url = `${mountPath()}/api/v1/me/books/${encodeURIComponent(id)}/file?format=epub`;
    const b = ePub(url);
    setBook(b);
    const r = b.renderTo(viewerRef.current, {
      width: '100%',
      height: '100%',
      spread: 'auto',
    });
    renditionRef.current = r;
    r.display();

    r.on('relocated', (location: { start?: { cfi?: string; percentage?: number } }) => {
      const cfi = location.start?.cfi ?? '';
      const percentage = location.start?.percentage ?? 0;
      setPct(Math.round(percentage * 100));
      void updateProgress(id, {
        last_cfi: cfi,
        read_progress: percentage,
        is_finished: percentage >= 0.98,
      }).catch(() => {});
    });
    return () => {
      r.destroy();
      b.destroy();
    };
  }, [id]);

  return (
    <div className="-mx-4 -my-2 flex h-[calc(100dvh-3.5rem)] flex-col md:-mx-6 lg:-mx-8">
      <header className="flex items-center gap-3 border-b border-border bg-card px-4 py-2">
        <Button asChild size="sm" variant="ghost">
          <Link to={`/${encodeURIComponent(id)}`}>
            <ChevronLeft className="mr-1 size-4" /> Back
          </Link>
        </Button>
        <div className="flex-1" />
        <span className="text-xs text-muted-foreground">{pct}%</span>
        <Button
          size="icon"
          variant="ghost"
          onClick={() => renditionRef.current?.prev()}
          aria-label="Previous"
        >
          <ArrowLeft className="size-4" />
        </Button>
        <Button
          size="icon"
          variant="ghost"
          onClick={() => renditionRef.current?.next()}
          aria-label="Next"
        >
          <ArrowRight className="size-4" />
        </Button>
      </header>
      <div ref={viewerRef} className="flex-1 overflow-hidden bg-background" />
      <div className="hidden">book-id:{book ? 'loaded' : 'pending'}</div>
    </div>
  );
}
