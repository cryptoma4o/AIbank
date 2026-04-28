FROM python:3.12-slim

WORKDIR /app

COPY pyproject.toml .
RUN pip install --no-cache-dir \
    "fastapi>=0.111" \
    "uvicorn[standard]>=0.30" \
    "httpx>=0.27" \
    "pydantic>=2.7"

COPY . .

EXPOSE 8106

CMD ["python", "-m", "uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8106"]
