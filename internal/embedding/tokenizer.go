package embedding

import (
	"bufio"
	"os"
	"strings"
	"unicode"
)

const (
	clsToken  = "[CLS]"
	sepToken  = "[SEP]"
	unkToken  = "[UNK]"
	maxSeqLen = 128
)

type Tokenizer struct {
	vocab map[string]int64
}

func NewTokenizer(vocabPath string) (*Tokenizer, error) {
	f, err := os.Open(vocabPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vocab := make(map[string]int64)
	scanner := bufio.NewScanner(f)
	var id int64
	for scanner.Scan() {
		vocab[scanner.Text()] = id
		id++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return &Tokenizer{vocab: vocab}, nil
}

type EncodedInput struct {
	InputIDs      []int64
	AttentionMask []int64
	TokenTypeIDs  []int64
}

func (t *Tokenizer) Encode(text string) EncodedInput {
	tokens := t.basicTokenize(text)
	wpTokens := t.wordPieceTokenize(tokens)

	if len(wpTokens) > maxSeqLen-2 {
		wpTokens = wpTokens[:maxSeqLen-2]
	}

	inputIDs := make([]int64, maxSeqLen)
	attentionMask := make([]int64, maxSeqLen)
	tokenTypeIDs := make([]int64, maxSeqLen)

	inputIDs[0] = t.vocab[clsToken]
	attentionMask[0] = 1

	for i, token := range wpTokens {
		id, ok := t.vocab[token]
		if !ok {
			id = t.vocab[unkToken]
		}
		inputIDs[i+1] = id
		attentionMask[i+1] = 1
	}

	sepIdx := len(wpTokens) + 1
	inputIDs[sepIdx] = t.vocab[sepToken]
	attentionMask[sepIdx] = 1

	return EncodedInput{
		InputIDs:      inputIDs,
		AttentionMask: attentionMask,
		TokenTypeIDs:  tokenTypeIDs,
	}
}

func (t *Tokenizer) basicTokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsSpace(r) {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		} else if unicode.IsPunct(r) || unicode.Is(unicode.Han, r) {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			tokens = append(tokens, string(r))
		} else {
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

func (t *Tokenizer) wordPieceTokenize(tokens []string) []string {
	var result []string
	for _, token := range tokens {
		result = append(result, t.wordPiece(token)...)
	}
	return result
}

func (t *Tokenizer) wordPiece(token string) []string {
	if _, ok := t.vocab[token]; ok {
		return []string{token}
	}

	var tokens []string
	remaining := token

	for len(remaining) > 0 {
		longest := ""
		for end := len(remaining); end > 0; end-- {
			substr := remaining[:end]
			if len(tokens) > 0 {
				substr = "##" + substr
			}
			if _, ok := t.vocab[substr]; ok {
				longest = substr
				break
			}
		}

		if longest == "" {
			tokens = append(tokens, unkToken)
			break
		}

		tokens = append(tokens, longest)
		prefix := longest
		if strings.HasPrefix(prefix, "##") {
			prefix = prefix[2:]
		}
		remaining = remaining[len(prefix):]
	}

	return tokens
}
