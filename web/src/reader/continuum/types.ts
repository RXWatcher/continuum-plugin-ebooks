export type ReaderConfigEnvelope = {
  book_id: string;
  config: Record<string, unknown>;
  updated_at?: string;
};

export type ReadestBookNote = {
  id: string;
  type: "bookmark" | "annotation" | "excerpt";
  cfi: string;
  xpointer0?: string;
  xpointer1?: string;
  page?: number;
  text?: string;
  style?: "highlight" | "underline" | "squiggly";
  color?: string;
  note: string;
  createdAt: number;
  updatedAt: number;
  deletedAt?: number | null;
};

export type ContinuumReaderBook = {
  id: string;
  hash: string;
  format: string;
  title: string;
  author: string;
  files: Array<{ format: string; mime_type: string; size_bytes: number }>;
  primaryLanguage?: string;
};
