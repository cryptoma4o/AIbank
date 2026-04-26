from __future__ import annotations
import base64


async def extract_text(content_base64: str | None, content_url: str | None) -> str:
    """
    Pre-MVP stub. Returns placeholder text.
    Production: route to Tesseract OCR sidecar or external OCR API.
    """
    if content_base64:
        try:
            raw = base64.b64decode(content_base64)
            # Try UTF-8 decode for text PDFs; fallback to placeholder
            return raw.decode("utf-8", errors="ignore")[:2000] or "[OCR_STUB: binary document]"
        except Exception:
            return "[OCR_STUB: decode error]"
    if content_url:
        return f"[OCR_STUB: would fetch {content_url}]"
    return "[OCR_STUB: no content provided]"
