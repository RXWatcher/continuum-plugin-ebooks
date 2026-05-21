import { useMemo, useState } from "react";

// Atmosphere mode for the ebooks reader. Mirrors the audiobooks
// plugin's overlay — soft animated gradient seeded from the active
// book's title, opt-in via localStorage. The overlay is non-
// interactive and sits behind the page content.

const STORAGE_KEY = "ebooks.atmosphere.enabled";

export function useAtmosphereEnabled(): [boolean, (v: boolean) => void] {
  const [enabled, setEnabled] = useState<boolean>(() => {
    try {
      return window.localStorage.getItem(STORAGE_KEY) === "true";
    } catch {
      return false;
    }
  });
  const set = (v: boolean) => {
    setEnabled(v);
    try {
      window.localStorage.setItem(STORAGE_KEY, v ? "true" : "false");
    } catch {
      /* private-mode */
    }
  };
  return [enabled, set];
}

function hashString(s: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return h >>> 0;
}

function hslColor(seed: number, offset: number): string {
  const hue = (seed + offset) % 360;
  return `hsl(${hue}, 45%, 30%)`;
}

export function AtmosphereOverlay({ seed }: { seed: string }) {
  const palette = useMemo(() => {
    const h = hashString(seed || "default");
    return {
      a: hslColor(h, 0),
      b: hslColor(h, 120),
      c: hslColor(h, 240),
    };
  }, [seed]);

  return (
    <div
      aria-hidden
      className="fixed inset-0 -z-10 overflow-hidden"
      style={{ pointerEvents: "none" }}
    >
      <div
        className="absolute -inset-[20%] opacity-30 blur-3xl"
        style={{
          background: `radial-gradient(circle at 20% 30%, ${palette.a}, transparent 40%),
                       radial-gradient(circle at 80% 70%, ${palette.b}, transparent 40%),
                       radial-gradient(circle at 50% 50%, ${palette.c}, transparent 50%)`,
          animation: "atmosphere-drift 60s ease-in-out infinite",
        }}
      />
      <style>{`
        @keyframes atmosphere-drift {
          0%   { transform: translate(0, 0) scale(1);   }
          25%  { transform: translate(2%, -1%) scale(1.05); }
          50%  { transform: translate(-1%, 2%) scale(0.98); }
          75%  { transform: translate(-2%, -2%) scale(1.03); }
          100% { transform: translate(0, 0) scale(1);   }
        }
      `}</style>
    </div>
  );
}

export function AtmosphereToggle() {
  const [enabled, setEnabled] = useAtmosphereEnabled();
  return (
    <button
      type="button"
      onClick={() => setEnabled(!enabled)}
      className="text-muted-foreground hover:bg-surface-hover hover:text-foreground inline-flex min-h-9 items-center gap-2 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
      title={enabled ? "Disable atmosphere mode" : "Enable atmosphere mode"}
    >
      <span
        className={enabled ? "bg-primary size-2 rounded-full" : "bg-muted-foreground/40 size-2 rounded-full"}
        aria-hidden
      />
      Atmosphere
    </button>
  );
}
