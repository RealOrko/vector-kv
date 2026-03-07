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
