FROM ghcr.io/astral-sh/uv:python3.12-bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends gcc g++ && rm -rf /var/lib/apt/lists/*
RUN groupadd -r simone && useradd -r -g simone simone

WORKDIR /app

COPY pyproject.toml README.md ./
COPY src/ ./src/
COPY .well-known/ ./.well-known/
COPY mcp-config.json ./

RUN uv pip install --system . && chown -R simone:simone /app

USER simone

EXPOSE 8234

CMD ["python", "-m", "uvicorn", "src.main:app", "--host", "0.0.0.0", "--port", "8234"]
