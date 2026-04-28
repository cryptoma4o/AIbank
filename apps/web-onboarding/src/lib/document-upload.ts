// document-upload — клиент для загрузки документов в document-service.
//
// document-service принимает multipart/form-data до 25MB с whitelist MIME
// (pdf/png/jpeg/tiff). См. services/document-service/internal/handler.
//
// Используем XMLHttpRequest вместо fetch ради прогресса (fetch ещё не имеет
// upload-progress в браузерах). При ошибке сети — XHR.onerror; при таймауте
// — XHR.ontimeout. Server-errors попадают в onloadend через статус-код.

export type UploadProgressEvent = {
  loaded: number;
  total: number;
  percent: number;
};

export type UploadResult = {
  documentId: string;
  filename: string;
  mimeType: string;
  size: number;
  sha256: string;
  uploadedAt: string;
};

export type UploadOptions = {
  applicationId: string;
  // Bearer token из identity-service (хранится в localStorage, см. lib/auth.ts).
  token: string;
  // Базовый URL document-service. По умолчанию /api прокси через Next.js.
  baseUrl?: string;
  onProgress?: (e: UploadProgressEvent) => void;
  // AbortController.signal для отмены пользователем.
  signal?: AbortSignal;
};

export type UploadError = {
  kind: "network" | "timeout" | "abort" | "server" | "validation";
  message: string;
  status?: number;
};

// Серверная валидация — но дублируем на клиенте для UX (мгновенный фидбек).
export const ALLOWED_MIME_TYPES = [
  "application/pdf",
  "image/png",
  "image/jpeg",
  "image/tiff",
] as const;

export const MAX_FILE_SIZE = 25 * 1024 * 1024; // 25MB

export function validateFile(file: File): UploadError | null {
  if (file.size === 0) {
    return { kind: "validation", message: "Файл пустой" };
  }
  if (file.size > MAX_FILE_SIZE) {
    return {
      kind: "validation",
      message: `Файл слишком большой (${formatBytes(file.size)}, лимит ${formatBytes(MAX_FILE_SIZE)})`,
    };
  }
  if (!(ALLOWED_MIME_TYPES as readonly string[]).includes(file.type)) {
    return {
      kind: "validation",
      message: `Неподдерживаемый формат "${file.type}". Допустимы: PDF, PNG, JPEG, TIFF`,
    };
  }
  return null;
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} Б`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} КБ`;
  return `${(bytes / 1024 / 1024).toFixed(1)} МБ`;
}

export function uploadDocument(
  file: File,
  options: UploadOptions,
): Promise<UploadResult> {
  return new Promise((resolve, reject) => {
    const validation = validateFile(file);
    if (validation) {
      reject(validation);
      return;
    }

    const baseUrl = options.baseUrl ?? "/api";
    const url = `${baseUrl}/v1/applications/${encodeURIComponent(options.applicationId)}/documents`;

    const xhr = new XMLHttpRequest();
    xhr.open("POST", url, true);
    xhr.setRequestHeader("Authorization", `Bearer ${options.token}`);
    xhr.timeout = 5 * 60 * 1000; // 5 минут — для медленного 4G
    xhr.responseType = "json";

    if (options.onProgress) {
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) {
          options.onProgress!({
            loaded: e.loaded,
            total: e.total,
            percent: Math.round((e.loaded / e.total) * 100),
          });
        }
      };
    }

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(xhr.response as UploadResult);
      } else {
        const message =
          (xhr.response && (xhr.response as { error?: string }).error) ||
          `Сервер вернул код ${xhr.status}`;
        reject({
          kind: "server",
          message,
          status: xhr.status,
        } satisfies UploadError);
      }
    };
    xhr.onerror = () =>
      reject({
        kind: "network",
        message: "Сетевая ошибка — проверьте соединение и попробуйте снова",
      } satisfies UploadError);
    xhr.ontimeout = () =>
      reject({
        kind: "timeout",
        message: "Превышено время ожидания. Попробуйте файл меньшего размера",
      } satisfies UploadError);

    if (options.signal) {
      options.signal.addEventListener("abort", () => {
        xhr.abort();
        reject({ kind: "abort", message: "Загрузка отменена" } satisfies UploadError);
      });
    }

    const form = new FormData();
    form.append("file", file, file.name);
    xhr.send(form);
  });
}
