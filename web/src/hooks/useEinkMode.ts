import { useEffect, useState } from "react";

// useEinkMode toggles a body-level "eink" class that other styles
// can hook to disable animations, gradients, and shadows — anything
// that ghosts on an e-ink display. The flag persists in
// localStorage so users on Kindle/Kobo browsers don't have to
// re-enable it every session.
//
// Stylesheet contract:
//   body.eink {
//     /* turn off transitions, animations, drop shadows */
//     animation-duration: 0s !important;
//     transition: none !important;
//     filter: none !important;
//   }
//   body.eink .blur-3xl,
//   body.eink .backdrop-blur-sm { filter: none !important; }
//
// The hook returns [enabled, setEnabled] so the toggle button
// renders in sync. Wire the toggle into reader settings.

const STORAGE_KEY = "ebooks.eink.enabled";
const CLASS_NAME = "eink";

export function useEinkMode(): [boolean, (v: boolean) => void] {
  const [enabled, setEnabled] = useState<boolean>(() => {
    try {
      return window.localStorage.getItem(STORAGE_KEY) === "true";
    } catch {
      return false;
    }
  });

  useEffect(() => {
    if (enabled) {
      document.body.classList.add(CLASS_NAME);
    } else {
      document.body.classList.remove(CLASS_NAME);
    }
    return () => {
      // Don't strip the class on unmount — other components may
      // still want it (e.g. nav rendered after the reader closes).
    };
  }, [enabled]);

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
