"use client";

// DocumentUploadModal — модальное окно загрузки документа с drag-and-drop,
// preview, progress-bar, multi-file selection и MIME-валидацией.
//
// Контракт: открывается из applications/[id]/page.tsx когда state ==
// "collecting_documents". После успешной загрузки вызывает onUploaded —
// родитель должен запустить refetch QUERY_APPLICATION чтобы DocumentList
// получил новую запись.

import { useCallback, useEffect, useRef, useState } from "react";

import {
  ALLOWED_MIME_TYPES,
  MAX_FILE_SIZE,
  formatBytes,
  uploadDocument,
  validateFile,
  type UploadError,
  type UploadResult,
} from "@/lib/document-upload";

type FileStatus =
  | { kind: "queued" }
  | { kind: "uploading"; percent: number }
  | { kind: "done"; result: UploadResult }
  | { kind: "error"; error: UploadError };

type FileEntry = {
  id: string;
  file: File;
  status: FileStatus;
};

interface Props {
  applicationId: string;
  token: string;
  isOpen: boolean;
  onClose: () => void;
  onUploaded?: (results: UploadResult[]) => void;
  // Для тестов — опциональный override для baseUrl document-service.
  baseUrl?: string;
}

export function DocumentUploadModal({
  applicationId,
  token,
  isOpen,
  onClose,
  onUploaded,
  baseUrl,
}: Props) {
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [isDragOver, setIsDragOver] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  // Сбрасываем state при закрытии чтобы при повторном открытии модалка была чистая.
  useEffect(() => {
    if (!isOpen) {
      setEntries([]);
      setIsDragOver(false);
    }
  }, [isOpen]);

  const addFiles = useCallback((files: FileList | File[]) => {
    const newEntries: FileEntry[] = [];
    for (const file of Array.from(files)) {
      const validation = validateFile(file);
      const id = `${file.name}-${file.size}-${file.lastModified}-${Math.random()}`;
      if (validation) {
        newEntries.push({ id, file, status: { kind: "error", error: validation } });
      } else {
        newEntries.push({ id, file, status: { kind: "queued" } });
      }
    }
    setEntries((prev) => [...prev, ...newEntries]);
  }, []);

  const startUpload = useCallback(
    async (entry: FileEntry) => {
      setEntries((prev) =>
        prev.map((e) =>
          e.id === entry.id
            ? { ...e, status: { kind: "uploading", percent: 0 } }
            : e,
        ),
      );
      try {
        const result = await uploadDocument(entry.file, {
          applicationId,
          token,
          baseUrl,
          onProgress: (p) =>
            setEntries((prev) =>
              prev.map((e) =>
                e.id === entry.id
                  ? { ...e, status: { kind: "uploading", percent: p.percent } }
                  : e,
              ),
            ),
        });
        setEntries((prev) =>
          prev.map((e) =>
            e.id === entry.id ? { ...e, status: { kind: "done", result } } : e,
          ),
        );
      } catch (err) {
        setEntries((prev) =>
          prev.map((e) =>
            e.id === entry.id
              ? { ...e, status: { kind: "error", error: err as UploadError } }
              : e,
          ),
        );
      }
    },
    [applicationId, token, baseUrl],
  );

  const startAllUploads = useCallback(async () => {
    const queued = entries.filter((e) => e.status.kind === "queued");
    // Загружаем последовательно — иначе можно положить network у клиента.
    for (const entry of queued) {
      // eslint-disable-next-line no-await-in-loop
      await startUpload(entry);
    }
    const finalState = entries.map((e) =>
      e.status.kind === "queued" ? e : e,
    );
    const successResults = finalState
      .map((e) => (e.status.kind === "done" ? e.status.result : null))
      .filter((r): r is UploadResult => r !== null);
    if (successResults.length > 0 && onUploaded) {
      onUploaded(successResults);
    }
  }, [entries, startUpload, onUploaded]);

  if (!isOpen) return null;

  const queuedCount = entries.filter((e) => e.status.kind === "queued").length;
  const uploadingCount = entries.filter((e) => e.status.kind === "uploading").length;
  const isBusy = uploadingCount > 0;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 px-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="upload-modal-title"
    >
      <div className="w-full max-w-2xl rounded-xl bg-white p-6 shadow-xl">
        <div className="flex items-start justify-between">
          <h2 id="upload-modal-title" className="text-lg font-semibold text-gray-900">
            Загрузка документов
          </h2>
          <button
            type="button"
            onClick={onClose}
            disabled={isBusy}
            className="text-gray-400 hover:text-gray-600 disabled:cursor-not-allowed disabled:opacity-50"
            aria-label="Закрыть"
          >
            ✕
          </button>
        </div>

        <p className="mt-2 text-sm text-gray-500">
          PDF, PNG, JPEG или TIFF, до {formatBytes(MAX_FILE_SIZE)} на файл.
        </p>

        {/* Drag-and-drop зона */}
        <div
          data-testid="upload-dropzone"
          onDragEnter={(e) => {
            e.preventDefault();
            setIsDragOver(true);
          }}
          onDragOver={(e) => {
            e.preventDefault();
            setIsDragOver(true);
          }}
          onDragLeave={(e) => {
            e.preventDefault();
            // Только если ушли за пределы dropzone (не в дочерний элемент).
            if (e.currentTarget.contains(e.relatedTarget as Node)) return;
            setIsDragOver(false);
          }}
          onDrop={(e) => {
            e.preventDefault();
            setIsDragOver(false);
            if (e.dataTransfer.files.length > 0) {
              addFiles(e.dataTransfer.files);
            }
          }}
          className={`mt-4 rounded-lg border-2 border-dashed p-8 text-center transition-colors ${
            isDragOver
              ? "border-primary bg-primary-50"
              : "border-gray-300 bg-gray-50"
          }`}
        >
          <p className="text-sm text-gray-700">
            Перетащите файлы сюда или{" "}
            <button
              type="button"
              onClick={() => inputRef.current?.click()}
              className="font-medium text-primary hover:underline"
            >
              выберите вручную
            </button>
          </p>
          <input
            ref={inputRef}
            type="file"
            accept={ALLOWED_MIME_TYPES.join(",")}
            multiple
            className="hidden"
            data-testid="upload-input"
            onChange={(e) => {
              if (e.target.files) {
                addFiles(e.target.files);
                // reset чтобы можно было выбрать тот же файл повторно после удаления.
                e.target.value = "";
              }
            }}
          />
        </div>

        {/* Список файлов */}
        {entries.length > 0 && (
          <ul className="mt-4 space-y-2 max-h-64 overflow-y-auto">
            {entries.map((entry) => (
              <li
                key={entry.id}
                data-testid="upload-entry"
                className="flex items-center justify-between gap-3 rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm"
              >
                <div className="min-w-0 flex-1">
                  <p className="truncate font-medium text-gray-900">{entry.file.name}</p>
                  <p className="text-xs text-gray-500">
                    {formatBytes(entry.file.size)} · {entry.file.type || "тип неизвестен"}
                  </p>
                  {entry.status.kind === "uploading" && (
                    <div
                      className="mt-1 h-1 overflow-hidden rounded-full bg-gray-100"
                      role="progressbar"
                      aria-valuenow={entry.status.percent}
                      aria-valuemin={0}
                      aria-valuemax={100}
                    >
                      <div
                        className="h-full bg-primary transition-all"
                        style={{ width: `${entry.status.percent}%` }}
                      />
                    </div>
                  )}
                  {entry.status.kind === "error" && (
                    <p className="mt-1 text-xs text-red-600">
                      {entry.status.error.message}
                    </p>
                  )}
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  {entry.status.kind === "queued" && (
                    <span className="text-xs text-gray-500">в очереди</span>
                  )}
                  {entry.status.kind === "uploading" && (
                    <span className="text-xs text-primary">{entry.status.percent}%</span>
                  )}
                  {entry.status.kind === "done" && (
                    <span className="text-xs text-green-600">✓ загружен</span>
                  )}
                  {entry.status.kind === "error" && (
                    <button
                      type="button"
                      onClick={() => startUpload({ ...entry, status: { kind: "queued" } })}
                      className="text-xs font-medium text-primary hover:underline"
                    >
                      Повторить
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={() =>
                      setEntries((prev) => prev.filter((e) => e.id !== entry.id))
                    }
                    disabled={entry.status.kind === "uploading"}
                    className="text-gray-400 hover:text-gray-600 disabled:opacity-50"
                    aria-label={`Убрать ${entry.file.name}`}
                  >
                    ✕
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}

        {/* Action bar */}
        <div className="mt-6 flex items-center justify-end gap-3">
          <button
            type="button"
            onClick={onClose}
            disabled={isBusy}
            className="rounded-lg border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
          >
            Закрыть
          </button>
          <button
            type="button"
            onClick={startAllUploads}
            disabled={queuedCount === 0 || isBusy}
            data-testid="upload-start"
            className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50"
          >
            {isBusy
              ? `Загрузка… (${uploadingCount})`
              : queuedCount > 0
                ? `Загрузить (${queuedCount})`
                : "Загрузить"}
          </button>
        </div>
      </div>
    </div>
  );
}
