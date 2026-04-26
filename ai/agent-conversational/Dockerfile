FROM python:3.12-slim

WORKDIR /app

COPY pyproject.toml .
RUN pip install --no-cache-dir .

COPY . .

EXPOSE 8106

CMD ["python", "-m", "uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8106"]
