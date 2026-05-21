import { useEffect, useRef } from "react";

// useScreenWakeLock keeps the device screen on while reading. The
// Wake Lock API ships in modern Chromium and Safari 16.4+; older
// browsers silently no-op. Pass false to release immediately (e.g.
// when the user navigates away from the reader).
//
// Reacquires on visibilitychange — browsers automatically release
// the lock when the tab loses focus, so we re-grab it when the user
// returns to keep the experience continuous.
export function useScreenWakeLock(active: boolean) {
  const sentinelRef = useRef<WakeLockSentinel | null>(null);

  useEffect(() => {
    if (!active) {
      sentinelRef.current?.release().catch(() => {});
      sentinelRef.current = null;
      return;
    }
    if (!("wakeLock" in navigator)) return;

    let cancelled = false;
    const acquire = async () => {
      try {
        const sentinel = await (navigator as Navigator & {
          wakeLock: { request: (type: "screen") => Promise<WakeLockSentinel> };
        }).wakeLock.request("screen");
        if (cancelled) {
          await sentinel.release().catch(() => {});
          return;
        }
        sentinelRef.current = sentinel;
      } catch {
        // Permissions/Policy denied — silently degrade. The user
        // can still read; they just have to keep the screen alive.
      }
    };
    void acquire();

    const onVisible = () => {
      if (document.visibilityState === "visible" && !sentinelRef.current) {
        void acquire();
      }
    };
    document.addEventListener("visibilitychange", onVisible);

    return () => {
      cancelled = true;
      document.removeEventListener("visibilitychange", onVisible);
      sentinelRef.current?.release().catch(() => {});
      sentinelRef.current = null;
    };
  }, [active]);
}

// Minimal WakeLockSentinel typing — TypeScript's lib.dom doesn't
// guarantee it on every target. Keeping the type local avoids a
// lib upgrade for a one-call hook.
interface WakeLockSentinel {
  release(): Promise<void>;
}
