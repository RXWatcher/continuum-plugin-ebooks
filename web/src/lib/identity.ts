import { fetchIdentity, type Identity } from "./api";

let cached: Identity | null = null;

export async function loadIdentity(): Promise<Identity | null> {
  if (cached) return cached;
  try {
    cached = await fetchIdentity();
    return cached;
  } catch {
    return null;
  }
}

export function currentUser(): Identity | null {
  return cached;
}
