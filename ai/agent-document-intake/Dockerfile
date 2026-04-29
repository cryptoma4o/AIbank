FROM python:3.12-slim

WORKDIR /app

# Сначала весь код, потом pip install — иначе setuptools не видит
# package directories (agent/, models/) при build wheel из pyproject.toml.
COPY . .

RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir .

EXPOSE 8101
CMD ["python", "-m", "uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8101"]
