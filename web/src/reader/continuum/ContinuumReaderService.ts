import {
  api,
  getBook,
  getReaderConfig,
  putReaderConfig,
  type EbookDetail,
} from "@/lib/api";

export class ContinuumReaderService {
  async loadBook(bookID: string): Promise<EbookDetail> {
    return getBook(bookID);
  }

  async loadBookContent(bookID: string, format: string): Promise<File> {
    const response = await api.fetchRaw(
      `/api/v1/me/books/${encodeURIComponent(bookID)}/file?format=${encodeURIComponent(format)}`,
    );
    if (!response.ok) {
      throw new Error(`Unable to load book file: ${response.status}`);
    }
    const blob = await response.blob();
    return new File([blob], `${bookID}.${format.toLowerCase()}`, {
      type: blob.type || "application/octet-stream",
    });
  }

  async loadBookConfig(bookID: string): Promise<Record<string, unknown>> {
    const envelope = await getReaderConfig(bookID);
    return envelope.config ?? {};
  }

  async saveBookConfig(
    bookID: string,
    config: Record<string, unknown>,
  ): Promise<void> {
    await putReaderConfig(bookID, config);
  }
}
