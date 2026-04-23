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

	ignore "github.com/sabhiram/go-gitignore"
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
  get <key> [-q <query>] [-m <meta>] [-k N]
                                    Search by query and/or metadata
  set <key> [value] [--meta <meta>] Store a value (reads stdin if no value given)
  delete <key>                      Delete a key
  index <key> <path> [--glob PAT]   Index a folder recursively
               [--meta M] [--dry-run] [--no-ignore]
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
		fmt.Fprintln(os.Stderr, "Usage: vector-kv get <key> [-q <query>] [-m <metadata>] [-k N]")
		os.Exit(1)
	}
	key := os.Args[2]
	var query, metadata string
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
		case "-m":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Missing value for -m")
				os.Exit(1)
			}
			i++
			metadata = args[i]
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
	if query == "" && metadata == "" {
		fmt.Fprintln(os.Stderr, "Usage: vector-kv get <key> [-q <query>] [-m <metadata>] [-k N]")
		os.Exit(1)
	}

	base := requireURL()
	params := url.Values{}
	if query != "" {
		params.Set("q", query)
	}
	if metadata != "" {
		params.Set("m", metadata)
	}
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
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: vector-kv set <key> [value] [--meta <metadata>]  (reads from stdin if no value given)")
		os.Exit(1)
	}
	key := os.Args[2]
	var value, metadata string
	var positionalArgs []string

	args := os.Args[3:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--meta":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Missing value for --meta")
				os.Exit(1)
			}
			i++
			metadata = args[i]
		default:
			positionalArgs = append(positionalArgs, args[i])
		}
	}

	if len(positionalArgs) > 0 {
		value = positionalArgs[0]
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}
		value = string(data)
	}
	if value == "" {
		fmt.Fprintln(os.Stderr, "Error: empty value")
		os.Exit(1)
	}

	cfg, _ := loadConfig()
	base := requireURL()
	reqURL := base + "/" + url.PathEscape(key)

	chunks := chunkText(value, cfg.chunkSize(), cfg.chunkOverlap())

	for i, chunk := range chunks {
		body := fmt.Sprintf("[#%d] %s", i+1, chunk)
		resp, err := doWithRetry(func() (*http.Response, error) {
			req, reqErr := http.NewRequest(http.MethodPost, reqURL, strings.NewReader(body))
			if reqErr != nil {
				return nil, reqErr
			}
			req.Header.Set("Content-Type", "text/plain")
			req.Header.Set("X-Chunk", strconv.Itoa(i+1))
			if metadata != "" {
				req.Header.Set("X-Metadata", metadata)
			}
			return http.DefaultClient.Do(req)
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			respBody, _ := io.ReadAll(resp.Body)
			fmt.Fprintf(os.Stderr, "Error (%d): %s\n", resp.StatusCode, strings.TrimSpace(string(respBody)))
			os.Exit(1)
		}
	}
	fmt.Printf("OK (%d chunks)\n", len(chunks))
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
		fmt.Fprintln(os.Stderr, "Usage: vector-kv index <key> <path> [--glob <pattern>] [--meta <metadata>] [--dry-run]")
		os.Exit(1)
	}
	key := os.Args[2]
	root := os.Args[3]
	globPattern := "*"
	metadata := ""
	dryRun := false
	noIgnore := false

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
		case "--meta":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Missing value for --meta")
				os.Exit(1)
			}
			i++
			metadata = args[i]
		case "--dry-run":
			dryRun = true
		case "--no-ignore":
			noIgnore = true
		default:
			fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", args[i])
			os.Exit(1)
		}
	}

	var gi *ignore.GitIgnore
	if !noIgnore {
		gitignorePath := filepath.Join(root, ".gitignore")
		if _, err := os.Stat(gitignorePath); err == nil {
			gi, _ = ignore.CompileIgnoreFile(gitignorePath)
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
		relPath, _ := filepath.Rel(root, path)
		if gi != nil && relPath != "." && gi.MatchesPath(relPath) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		baseName := filepath.Base(path)
		matched := false
		var matchErr error
		for _, pat := range strings.Split(globPattern, ",") {
			m, err := filepath.Match(pat, baseName)
			if err != nil {
				matchErr = err
				break
			}
			if m {
				matched = true
				break
			}
		}
		if matchErr != nil || !matched {
			return nil
		}

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

		content := strings.ToValidUTF8(string(data), "")
		content = strings.ReplaceAll(content, "\x00", "")
		chunks := chunkText(content, chunkSize, chunkOverlap)

		for i, chunk := range chunks {
			body := fmt.Sprintf("[%s] [#%d] %s", relPath, i+1, chunk)
			reqURL := base + "/" + url.PathEscape(key)
			resp, postErr := doWithRetry(func() (*http.Response, error) {
				req, reqErr := http.NewRequest(http.MethodPost, reqURL, strings.NewReader(body))
				if reqErr != nil {
					return nil, reqErr
				}
				req.Header.Set("Content-Type", "text/plain")
				req.Header.Set("X-Chunk", strconv.Itoa(i+1))
				chunkMeta := metadata
				if chunkMeta == "" {
					chunkMeta = relPath
				}
				req.Header.Set("X-Metadata", chunkMeta)
				return http.DefaultClient.Do(req)
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
