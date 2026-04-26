"use client";
import { useState, useRef } from "react";

interface DocumentUploadProps {
  label: string;
  accept?: string;
  onUpload?: (file: File) => void;
}

export function DocumentUpload({ label, accept = ".pdf,.jpg,.jpeg,.png", onUpload }: DocumentUploadProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [fileName, setFileName] = useState<string | null>(null);
  const [dragging, setDragging] = useState(false);

  function handleFile(file: File) {
    setFileName(file.name);
    onUpload?.(file);
  }

  function handleChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (file) handleFile(file);
  }

  function handleDrop(e: React.DragEvent) {
    e.preventDefault();
    setDragging(false);
    const file = e.dataTransfer.files?.[0];
    if (file) handleFile(file);
  }

  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-sm font-medium text-gray-700">{label}</span>
      <div
        onClick={() => inputRef.current?.click()}
        onDragOver={e => { e.preventDefault(); setDragging(true); }}
        onDragLeave={() => setDragging(false)}
        onDrop={handleDrop}
        className={`border-2 border-dashed rounded-xl px-6 py-8 flex flex-col items-center gap-2 cursor-pointer transition-colors
          ${dragging ? "border-primary bg-primary/5" : "border-gray-300 hover:border-gray-400 hover:bg-gray-50"}`}
      >
        <div className="w-10 h-10 rounded-full bg-gray-100 flex items-center justify-center text-gray-400 text-xl">
          ↑
        </div>
        {fileName ? (
          <span className="text-sm text-gray-700 font-medium">{fileName}</span>
        ) : (
          <>
            <span className="text-sm text-gray-600">Перетащите файл или <span className="text-primary font-medium">выберите</span></span>
            <span className="text-xs text-gray-400">PDF, JPG, PNG — до 10 МБ</span>
          </>
        )}
        <input ref={inputRef} type="file" accept={accept} className="hidden" onChange={handleChange} />
      </div>
    </div>
  );
}
