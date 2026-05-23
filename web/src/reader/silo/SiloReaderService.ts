import {
  api,
  getBook,
  getReaderConfig,
  putReaderConfig,
  type EbookDetail,
} from "@/lib/api";

export class SiloReaderService {
  async loadBook(bookID: string): Promise<EbookDetail> {
    return getBook(bookID);
  }

  /**
   * Fetch the raw bytes for one format of a book.
   *
   * fileUrl, when provided, is the portal-signed URL from detail.files[].url —
   * it carries a short-TTL token in ?token= so the request can succeed
   * without sending an Authorization header. The caller (ReadestLiteReader)
   * looks it up from the already-loaded book detail.
   *
   * When fileUrl is empty (older backends, missing detail), the service falls
   * back to the portal proxy endpoint, which uses Authorization on the api
   * client and proxies bytes from the backend itself.
   */
  async loadBookContent(
    bookID: string,
    format: string,
    fileUrl?: string,
  ): Promise<File> {
    const response = fileUrl
      ? await fetch(fileUrl)
      : await api.fetchRaw(
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
