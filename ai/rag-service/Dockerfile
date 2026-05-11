FROM python:3.12-slim

WORKDIR /app

COPY pyproject.toml ./
# NumPy 2.x требует x86-64-v2 SIMD ("baseline optimizations: X86_V2"). На
# нашем staging-сервере QEMU Virtual CPU 2.5+ = x86-64-v1, поэтому Qdrant
# init падает и rag-service делает fallback на in-memory store.
# Пиним numpy<2 (1.26.x) — совместим с baseline x86-v1.
RUN pip install --no-cache-dir --upgrade pip \
    && pip install --no-cache-dir \
        "numpy<2.0" \
        "fastapi>=0.111" \
        "uvicorn[standard]>=0.30" \
        "httpx>=0.27" \
        "pydantic>=2.7" \
        "qdrant-client>=1.9,<1.10"

COPY . .

EXPOSE 8105

CMD ["python", "-m", "uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8105"]
