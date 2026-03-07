FROM golang:1.22-bookworm AS builder

ARG ORT_VERSION=1.20.1
ARG TARGETARCH

WORKDIR /download
RUN apt-get update && apt-get install -y --no-install-recommends curl && \
    if [ "$TARGETARCH" = "arm64" ]; then \
      ORT_ARCH="aarch64"; \
    else \
      ORT_ARCH="x64"; \
    fi && \
    curl -L "https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/onnxruntime-linux-${ORT_ARCH}-${ORT_VERSION}.tgz" | \
    tar xz && \
    cp onnxruntime-linux-${ORT_ARCH}-${ORT_VERSION}/lib/libonnxruntime.so* /usr/lib/ && \
    ldconfig

RUN mkdir -p /model && \
    curl -L -o /model/model.onnx \
      "https://huggingface.co/Xenova/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx" && \
    curl -L -o /model/vocab.txt \
      "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/vocab.txt"

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /vector-kv ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/lib/libonnxruntime.so* /usr/lib/
RUN ldconfig
COPY --from=builder /model/ /model/
COPY --from=builder /vector-kv /usr/local/bin/vector-kv

USER 1000:1000
EXPOSE 8080
CMD ["vector-kv"]
