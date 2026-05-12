import type { MediaDetails } from '../lib/tmdb';
import type { PresenceEntry } from '../lib/api';

export interface PreloadShape {
  mediaType: 'movie' | 'tv';
  details: MediaDetails;
  presence?: PresenceEntry;
}

declare global {
  interface Window {
    __PRELOAD__?: PreloadShape;
  }
}
