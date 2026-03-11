package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"vector-kv/internal/embedding"
	"vector-kv/internal/store"
)

func main() {
	dbURL := getEnv("DATABASE_URL", "postgres://vectorkv:vectorkv@localhost:5432/vectorkv?sslmode=disable")
	modelPath := getEnv("MODEL_PATH", "/model/model.onnx")
	vocabPath := getEnv("VOCAB_PATH", "/model/vocab.txt")
	ortLibPath := getEnv("ORT_LIB_PATH", "/usr/lib/libonnxruntime.so")
	addr := getEnv("LISTEN_ADDR", ":8080")

	s, err := store.New(dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer s.Close()

	e, err := embedding.NewEmbedder(modelPath, vocabPath, ortLibPath)
	if err != nil {
		log.Fatalf("Failed to initialize embedder: %v", err)
	}
	defer e.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		keys, err := s.Keys(r.Context())
		if err != nil {
			http.Error(w, "keys failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if keys == nil {
			keys = []string{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(keys)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/")
		if key == "" {
			http.Error(w, "key required", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodGet:
			handleGet(w, r, key, s, e)
		case http.MethodPost:
			handlePost(w, r, key, s, e)
		case http.MethodDelete:
			handleDelete(w, r, key, s)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	log.Printf("Listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func handleGet(w http.ResponseWriter, r *http.Request, key string, s *store.Store, e *embedding.Embedder) {
	query := r.URL.Query().Get("q")
	metadata := r.URL.Query().Get("m")

	if query == "" && metadata == "" {
		http.Error(w, "q and/or m parameter required", http.StatusBadRequest)
		return
	}

	k := 10
	if kStr := r.URL.Query().Get("k"); kStr != "" {
		if kVal, err := strconv.Atoi(kStr); err == nil && kVal > 0 {
			k = kVal
		}
	}

	var results []store.Result
	var err error

	if query == "" {
		results, err = s.QueryByMetadata(r.Context(), key, metadata, k)
	} else {
		vec, embedErr := e.Embed(query)
		if embedErr != nil {
			http.Error(w, "embedding failed: "+embedErr.Error(), http.StatusInternalServerError)
			return
		}
		results, err = s.Query(r.Context(), key, vec, metadata, k)
	}

	if err != nil {
		http.Error(w, "query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func handlePost(w http.ResponseWriter, r *http.Request, key string, s *store.Store, e *embedding.Embedder) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	content := string(body)
	metadata := r.Header.Get("X-Metadata")
	chunk := 0
	if chunkStr := r.Header.Get("X-Chunk"); chunkStr != "" {
		if v, parseErr := strconv.Atoi(chunkStr); parseErr == nil {
			chunk = v
		}
	}

	vec, err := e.Embed(content)
	if err != nil {
		http.Error(w, "embedding failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.Insert(r.Context(), key, content, metadata, chunk, vec); err != nil {
		http.Error(w, "insert failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func handleDelete(w http.ResponseWriter, r *http.Request, key string, s *store.Store) {
	if err := s.Delete(r.Context(), key); err != nil {
		http.Error(w, "delete failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
