FROM golang:1.22.10-bookworm AS builder

SHELL ["/bin/bash", "-c"]

RUN apt-get update -y && apt-get install -y --no-install-recommends \
    ca-certificates \
    pkg-config \
    gcc g++ \
    libgtk-3-dev \
    libwebkit2gtk-4.1-dev \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# webview_go asks pkg-config for webkit2gtk-4.0; Debian bookworm ships 4.1.
RUN if ! pkg-config --exists webkit2gtk-4.0; then \
      pc_paths="$(pkg-config --variable pc_path pkg-config)"; \
      alias_dir="/tmp/pkgconfig"; \
      mkdir -p "${alias_dir}"; \
      IFS=':' read -ra dirs <<< "${pc_paths}"; \
      for d in "${dirs[@]}"; do \
        if [[ -f "${d}/webkit2gtk-4.1.pc" ]]; then \
          ln -sf "${d}/webkit2gtk-4.1.pc" "${alias_dir}/webkit2gtk-4.0.pc"; \
          break; \
        fi; \
      done; \
      export PKG_CONFIG_PATH="${alias_dir}:${PKG_CONFIG_PATH:-}"; \
    fi; \
    pkg-config --exists webkit2gtk-4.0

# Copy module metadata + vendored deps first for better layer caching.
COPY go.mod go.sum ./
COPY vendor/ ./vendor/

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY config/ ./config/

ENV CGO_ENABLED=1

RUN if ! pkg-config --exists webkit2gtk-4.0; then export PKG_CONFIG_PATH="/tmp/pkgconfig:${PKG_CONFIG_PATH:-}"; fi && \
    go build -mod=vendor -trimpath -buildvcs=false -ldflags "-s -w" -o /out/pterminal ./cmd/pterminal


FROM ubuntu:24.04 AS runtime

RUN apt-get update -y && apt-get install -y --no-install-recommends \
    ca-certificates \
    libgtk-3-0 \
    libwebkit2gtk-4.1-0 \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/pterminal /usr/local/bin/pterminal

ENV HOME=/home/pterminal
RUN mkdir -p /home/pterminal/.config/pterminal /home/pterminal/Downloads && chmod -R 0777 /home/pterminal

ENTRYPOINT ["/usr/local/bin/pterminal"]
