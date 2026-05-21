import { useCallback, useEffect, useRef, useState } from "react";

// Minimal Web Speech TTS controller for the ebook reader. Web Speech
// is the only no-network option that works in browsers today; Edge
// TTS / native TTS would need a backend bridge. Speak / pause /
// resume / stop + voice selection are the user-visible knobs.
//
// MediaSession integration: when TTS is speaking, register handlers
// so the OS lock-screen play/pause/stop controls drive the
// SpeechSynthesis state instead of (or alongside) the audiobook
// player. The audiobooks-side player owns MediaSession when active;
// TTS yields when an audio session is in flight.

export type TTSState = "idle" | "speaking" | "paused";

export interface TTSOptions {
  rate?: number; // 0.5–2.0, default 1.0
  pitch?: number; // 0–2.0, default 1.0
  volume?: number; // 0–1.0, default 1.0
  voiceURI?: string; // pick from voices()
  lang?: string; // BCP-47 language tag
}

// useTTS exposes a speak() that breaks text into chunks (sentence
// boundary), enqueues them, and tracks the playback state. Chunking
// lets us skip ahead / repeat the current sentence later without
// re-issuing the whole utterance.
export function useTTS() {
  const [state, setState] = useState<TTSState>("idle");
  const [voices, setVoices] = useState<SpeechSynthesisVoice[]>([]);
  const currentUtter = useRef<SpeechSynthesisUtterance | null>(null);

  // Voices populate asynchronously on Chromium; subscribe to the
  // voiceschanged event so the list reflects what the browser
  // ultimately offers.
  useEffect(() => {
    if (typeof window === "undefined" || !("speechSynthesis" in window)) return;
    const update = () => setVoices(window.speechSynthesis.getVoices());
    update();
    window.speechSynthesis.addEventListener("voiceschanged", update);
    return () => window.speechSynthesis.removeEventListener("voiceschanged", update);
  }, []);

  const stop = useCallback(() => {
    if (!("speechSynthesis" in window)) return;
    window.speechSynthesis.cancel();
    currentUtter.current = null;
    setState("idle");
  }, []);

  const pause = useCallback(() => {
    if (!("speechSynthesis" in window)) return;
    window.speechSynthesis.pause();
    setState("paused");
  }, []);

  const resume = useCallback(() => {
    if (!("speechSynthesis" in window)) return;
    window.speechSynthesis.resume();
    setState("speaking");
  }, []);

  const speak = useCallback(
    (text: string, opts: TTSOptions = {}) => {
      if (!("speechSynthesis" in window)) return;
      window.speechSynthesis.cancel();
      const cleaned = text.replace(/\s+/g, " ").trim();
      if (!cleaned) return;

      // Sentence-split — keep clauses under ~200 chars so the
      // utterance loop has a chance to recover from a stuck
      // Chromium TTS queue (a known issue with long single-shot
      // utterances).
      const sentences = cleaned.match(/[^.!?]+[.!?]+|\S[^.!?]*$/g) ?? [cleaned];
      const queue: string[] = [];
      for (const s of sentences) {
        let chunk = s.trim();
        while (chunk.length > 200) {
          const cut = chunk.lastIndexOf(" ", 200);
          queue.push(chunk.slice(0, cut > 0 ? cut : 200));
          chunk = chunk.slice(cut > 0 ? cut + 1 : 200);
        }
        if (chunk) queue.push(chunk);
      }

      const speakNext = () => {
        const next = queue.shift();
        if (!next) {
          setState("idle");
          currentUtter.current = null;
          installMediaSession(null, { pause, resume, stop });
          return;
        }
        const u = new SpeechSynthesisUtterance(next);
        u.rate = opts.rate ?? 1;
        u.pitch = opts.pitch ?? 1;
        u.volume = opts.volume ?? 1;
        if (opts.lang) u.lang = opts.lang;
        if (opts.voiceURI) {
          const v = window.speechSynthesis.getVoices().find((vv) => vv.voiceURI === opts.voiceURI);
          if (v) u.voice = v;
        }
        u.onend = speakNext;
        u.onerror = () => {
          // Surface errors as silent skip — the speech engine
          // sometimes throws "interrupted" when the user
          // cancels mid-sentence.
          speakNext();
        };
        currentUtter.current = u;
        window.speechSynthesis.speak(u);
      };

      setState("speaking");
      installMediaSession({ title: "Read aloud" }, { pause, resume, stop });
      speakNext();
    },
    [pause, resume, stop],
  );

  // Clean up on unmount — long-running TTS that survives a route
  // change would otherwise keep talking after the page is gone.
  useEffect(() => {
    return () => {
      if ("speechSynthesis" in window) window.speechSynthesis.cancel();
    };
  }, []);

  return { state, voices, speak, pause, resume, stop };
}

// installMediaSession wires the OS lock-screen controls to the
// passed callbacks. nil metadata releases the session so the
// audiobook player's MediaSession (if any) takes over again.
function installMediaSession(
  meta: { title?: string } | null,
  handlers: { pause: () => void; resume: () => void; stop: () => void },
) {
  if (typeof navigator === "undefined" || !("mediaSession" in navigator)) return;
  if (meta) {
    navigator.mediaSession.metadata = new window.MediaMetadata({
      title: meta.title ?? "Read aloud",
    });
    navigator.mediaSession.playbackState = "playing";
    navigator.mediaSession.setActionHandler("play", handlers.resume);
    navigator.mediaSession.setActionHandler("pause", handlers.pause);
    navigator.mediaSession.setActionHandler("stop", handlers.stop);
  } else {
    navigator.mediaSession.metadata = null;
    navigator.mediaSession.playbackState = "none";
    navigator.mediaSession.setActionHandler("play", null);
    navigator.mediaSession.setActionHandler("pause", null);
    navigator.mediaSession.setActionHandler("stop", null);
  }
}
