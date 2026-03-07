package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "vector-kv")
}

func configPath() string {
	return filepath.Join(configDir(), "config.json")
}

type config struct {
	URL          string `json:"url"`
	ChunkSize    int    `json:"chunk_size,omitempty"`
	ChunkOverlap int    `json:"chunk_overlap,omitempty"`
}

func (c config) chunkSize() int {
	if c.ChunkSize > 0 {
		return c.ChunkSize
	}
	return 800
}

func (c config) chunkOverlap() int {
	if c.ChunkOverlap > 0 {
		return c.ChunkOverlap
	}
	return 200
}

func loadConfig() (config, error) {
	var c config
	data, err := os.ReadFile(configPath())
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(data, &c)
	return c, err
}

func saveConfig(c config) error {
	if err := os.MkdirAll(configDir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0644)
}

func requireURL() string {
	c, err := loadConfig()
	if err != nil || c.URL == "" {
		fmt.Fprintln(os.Stderr, "No URL configured. Run: vector-kv config set-url <url>")
		os.Exit(1)
	}
	return strings.TrimRight(c.URL, "/")
}

// doWithRetry executes an HTTP request function with retries on network errors
// and 5xx responses. It retries up to 3 times with exponential backoff.
func doWithRetry(fn func() (*http.Response, error)) (*http.Response, error) {
	const maxRetries = 3
	backoff := 1 * time.Second

	resp, err := fn()
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err == nil && resp.StatusCode < 500 {
			return resp, nil
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Retry %d/%d: %v\n", attempt, maxRetries, err)
		} else {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			fmt.Fprintf(os.Stderr, "Retry %d/%d: server error %d: %s\n", attempt, maxRetries, resp.StatusCode, strings.TrimSpace(string(body)))
		}
		time.Sleep(backoff)
		backoff *= 2
		resp, err = fn()
	}
	return resp, err
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: vector-kv <command> [arguments]

Commands:
  config set-url <url>              Set the server URL
  config set-chunk-size <N>         Set chunk size (default: 800)
  config set-chunk-overlap <N>      Set chunk overlap (default: 200)
  config show                       Show current configuration
  keys                              List all keys
  get <key> -q <query> [-k N]       Semantic search within a key
  set <key> <value>                 Store a value under a key
  delete <key>                      Delete a key
  index <key> <path> [--glob PAT]   Index a folder recursively
                     [--dry-run]
`)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	switch os.Args[1] {
	case "config":
		cmdConfig()
	case "keys":
		cmdKeys()
	case "get":
		cmdGet()
	case "set":
		cmdSet()
	case "delete":
		cmdDelete()
	case "index":
		cmdIndex()
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		usage()
	}
}

func cmdConfig() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: vector-kv config <set-url|set-chunk-size|set-chunk-overlap|show>")
		os.Exit(1)
	}
	switch os.Args[2] {
	case "set-url":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: vector-kv config set-url <url>")
			os.Exit(1)
		}
		c, _ := loadConfig()
		c.URL = os.Args[3]
		if err := saveConfig(c); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("URL set to %s\n", c.URL)
	case "set-chunk-size":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: vector-kv config set-chunk-size <N>")
			os.Exit(1)
		}
		val, err := strconv.Atoi(os.Args[3])
		if err != nil || val < 1 {
			fmt.Fprintln(os.Stderr, "Invalid chunk size")
			os.Exit(1)
		}
		c, _ := loadConfig()
		c.ChunkSize = val
		if err := saveConfig(c); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Chunk size set to %d\n", val)
	case "set-chunk-overlap":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: vector-kv config set-chunk-overlap <N>")
			os.Exit(1)
		}
		val, err := strconv.Atoi(os.Args[3])
		if err != nil || val < 0 {
			fmt.Fprintln(os.Stderr, "Invalid chunk overlap")
			os.Exit(1)
		}
		c, _ := loadConfig()
		c.ChunkOverlap = val
		if err := saveConfig(c); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Chunk overlap set to %d\n", val)
	case "show":
		c, err := loadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "No configuration found. Run: vector-kv config set-url <url>")
			os.Exit(1)
		}
		fmt.Printf("url:           %s\n", c.URL)
		fmt.Printf("chunk_size:    %d\n", c.chunkSize())
		fmt.Printf("chunk_overlap: %d\n", c.chunkOverlap())
	default:
		fmt.Fprintf(os.Stderr, "Unknown config command: %s\n", os.Args[2])
		os.Exit(1)
	}
}

func cmdKeys() {
	base := requireURL()
	resp, err := doWithRetry(func() (*http.Response, error) {
		return http.Get(base + "/keys")
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Error (%d): %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		os.Exit(1)
	}

	var keys []string
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding response: %v\n", err)
		os.Exit(1)
	}
	for _, k := range keys {
		fmt.Println(k)
	}
}

func cmdGet() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: vector-kv get <key> -q <query> [-k N]")
		os.Exit(1)
	}
	key := os.Args[2]
	var query string
	k := 0
	args := os.Args[3:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-q":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Missing value for -q")
				os.Exit(1)
			}
			i++
			query = args[i]
		case "-k":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Missing value for -k")
				os.Exit(1)
			}
			i++
			val, err := strconv.Atoi(args[i])
			if err != nil || val < 1 {
				fmt.Fprintln(os.Stderr, "Invalid value for -k")
				os.Exit(1)
			}
			k = val
		default:
			fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", args[i])
			os.Exit(1)
		}
	}
	if query == "" {
		fmt.Fprintln(os.Stderr, "Usage: vector-kv get <key> -q <query> [-k N]")
		os.Exit(1)
	}

	base := requireURL()
	params := url.Values{"q": {query}}
	if k > 0 {
		params.Set("k", strconv.Itoa(k))
	}
	reqURL := base + "/" + url.PathEscape(key) + "?" + params.Encode()

	resp, err := doWithRetry(func() (*http.Response, error) {
		return http.Get(reqURL)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Error (%d): %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		os.Exit(1)
	}

	var results []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding response: %v\n", err)
		os.Exit(1)
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(out))
}

func cmdSet() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "Usage: vector-kv set <key> <value>")
		os.Exit(1)
	}
	key := os.Args[2]
	value := os.Args[3]

	base := requireURL()
	reqURL := base + "/" + url.PathEscape(key)

	resp, err := doWithRetry(func() (*http.Response, error) {
		return http.Post(reqURL, "text/plain", strings.NewReader(value))
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Error (%d): %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		os.Exit(1)
	}
	fmt.Println("OK")
}

func cmdDelete() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: vector-kv delete <key>")
		os.Exit(1)
	}
	key := os.Args[2]

	base := requireURL()
	reqURL := base + "/" + url.PathEscape(key)

	resp, err := doWithRetry(func() (*http.Response, error) {
		req, reqErr := http.NewRequest(http.MethodDelete, reqURL, nil)
		if reqErr != nil {
			return nil, reqErr
		}
		return http.DefaultClient.Do(req)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Error (%d): %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		os.Exit(1)
	}
	fmt.Println("Deleted")
}

func isBinary(data []byte) bool {
	check := data
	if len(check) > 512 {
		check = check[:512]
	}
	for _, b := range check {
		if b == 0 {
			return true
		}
	}
	return false
}

func chunkText(text string, size, overlap int) []string {
	runes := []rune(text)
	if len(runes) <= size {
		return []string{text}
	}
	var chunks []string
	step := size - overlap
	if step < 1 {
		step = 1
	}
	for i := 0; i < len(runes); i += step {
		end := i + size
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
		if end == len(runes) {
			break
		}
	}
	return chunks
}

func cmdIndex() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "Usage: vector-kv index <key> <path> [--glob <pattern>] [--dry-run]")
		os.Exit(1)
	}
	key := os.Args[2]
	root := os.Args[3]
	globPattern := "*"
	dryRun := false

	args := os.Args[4:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--glob":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Missing value for --glob")
				os.Exit(1)
			}
			i++
			globPattern = args[i]
		case "--dry-run":
			dryRun = true
		default:
			fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", args[i])
			os.Exit(1)
		}
	}

	cfg, _ := loadConfig()
	base := requireURL()
	chunkSize := cfg.chunkSize()
	chunkOverlap := cfg.chunkOverlap()

	var indexed, errors int

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		matched, matchErr := filepath.Match(globPattern, filepath.Base(path))
		if matchErr != nil || !matched {
			return nil
		}

		relPath, _ := filepath.Rel(root, path)

		if dryRun {
			fmt.Println(relPath)
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not read %s: %v\n", relPath, readErr)
			errors++
			return nil
		}

		if isBinary(data) {
			return nil
		}

		content := string(data)
		chunks := chunkText(content, chunkSize, chunkOverlap)

		for _, chunk := range chunks {
			body := fmt.Sprintf("[%s] %s", relPath, chunk)
			reqURL := base + "/" + url.PathEscape(key)
			resp, postErr := doWithRetry(func() (*http.Response, error) {
				return http.Post(reqURL, "text/plain", strings.NewReader(body))
			})
			if postErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to index %s: %v\n", relPath, postErr)
				errors++
				return nil
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusCreated {
				fmt.Fprintf(os.Stderr, "Warning: server error indexing %s: %d\n", relPath, resp.StatusCode)
				errors++
				return nil
			}
		}

		fmt.Printf("Indexed: %s (%d chunks)\n", relPath, len(chunks))
		indexed++
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error walking directory: %v\n", err)
		os.Exit(1)
	}

	if !dryRun {
		fmt.Printf("\nIndexed %d files (%d errors)\n", indexed, errors)
	}
}
