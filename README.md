# 🔑 vector-kv

A vector key-value store with semantic search. Stores text content with vector embeddings and retrieves entries by cosine similarity. 🧠

Built with Go, ONNX Runtime (all-MiniLM-L6-v2), and PostgreSQL with pgvector.

## 📡 API

### 📝 Store content

```
POST /:key
Body: plain text content
```

Embeds the body text and stores it under the given key. Returns `201 Created`.

### 🔍 Query by similarity

```
GET /:key?q=<query>&k=<limit>
```

Embeds the query and returns the most similar entries for that key, ordered by cosine distance. `k` defaults to 10 if not provided.

```json
[
  {"content": "some stored text", "distance": 0.25},
  {"content": "another entry", "distance": 0.61}
]
```

### 🗑️ Delete entries

```
DELETE /:key
```

Deletes all entries for the given key. Returns `204 No Content`.

### 🗂️ List keys

```
GET /keys
```

Returns a JSON array of all distinct keys that have stored entries.

```json
["my-docs", "notes", "recipes"]
```

## 🚀 Build and Deploy

Requires Docker and a local Kubernetes cluster with Helm.

```bash
# Build and push the Docker image to the local registry
bin/build

# Deploy to the vector-kv namespace
bin/deploy
```

The service is exposed via NodePort on port **30080**.

```bash
# Store content
curl -X POST http://<node-ip>:30080/my-key -d "Some text to store"

# Query (top 5 results)
curl "http://<node-ip>:30080/my-key?q=search+terms&k=5"

# List all keys
curl http://<node-ip>:30080/keys

# Delete a key
curl -X DELETE http://<node-ip>:30080/my-key
```

## 💻 CLI

A command-line client for interacting with the vector-kv server.

### Install

```bash
bin/install-cli
```

Builds and installs `vector-kv` to `~/.local/bin/`.

### Setup

```bash
vector-kv config set-url http://<node-ip>:30080
```

### Commands

```bash
# List all keys
vector-kv keys

# Store a value (inline or from stdin)
vector-kv set my-key "Some text to store"
cat file.txt | vector-kv set my-key

# Semantic search (top 5 results)
vector-kv get my-key -q "search terms" -k 5

# Delete a key
vector-kv delete my-key

# Index a folder (with optional glob filter and dry-run)
vector-kv index my-key ./docs --glob "*.md" --dry-run
vector-kv index my-key ./docs --glob "*.md"

# Configure chunking (applies to set and index)
vector-kv config set-chunk-size 800
vector-kv config set-chunk-overlap 200

# Show current config
vector-kv config show
```

All HTTP requests retry up to 3 times with exponential backoff on network errors and 5xx server responses.

## 🏗️ Architecture

- 🖥️ **Go HTTP service** — handles API requests, runs embeddings in-process via ONNX Runtime, stores/queries vectors in PostgreSQL
- 🤖 **all-MiniLM-L6-v2** — sentence transformer model (384-dimensional embeddings), loaded as ONNX and executed with `yalue/onnxruntime_go`
- 🐘 **PostgreSQL + pgvector** — vector storage with HNSW index for fast approximate nearest neighbor search (50GB storage)
- ✂️ **Pure Go tokenizer** — WordPiece tokenizer for BERT-style tokenization, no CGO dependency

## ⚙️ Configuration

Environment variables (with defaults):

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | `postgres://vectorkv:vectorkv@localhost:5432/vectorkv?sslmode=disable` | PostgreSQL connection string |
| `MODEL_PATH` | `/model/model.onnx` | Path to ONNX model file |
| `VOCAB_PATH` | `/model/vocab.txt` | Path to WordPiece vocabulary |
| `ORT_LIB_PATH` | `/usr/lib/libonnxruntime.so` | Path to ONNX Runtime shared library |
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
