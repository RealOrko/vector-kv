package embedding

import (
	"fmt"
	"math"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

const embeddingDim = 384

type Embedder struct {
	mu        sync.Mutex
	tokenizer *Tokenizer
	session   *ort.AdvancedSession
	inputIDs  *ort.Tensor[int64]
	mask      *ort.Tensor[int64]
	typeIDs   *ort.Tensor[int64]
	output    *ort.Tensor[float32]
}

func NewEmbedder(modelPath, vocabPath, ortLibPath string) (*Embedder, error) {
	ort.SetSharedLibraryPath(ortLibPath)
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("init ort environment: %w", err)
	}

	tokenizer, err := NewTokenizer(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}

	shape := ort.NewShape(1, maxSeqLen)
	outputShape := ort.NewShape(1, maxSeqLen, embeddingDim)

	inputIDs, err := ort.NewTensor(shape, make([]int64, maxSeqLen))
	if err != nil {
		return nil, fmt.Errorf("create input_ids tensor: %w", err)
	}
	mask, err := ort.NewTensor(shape, make([]int64, maxSeqLen))
	if err != nil {
		return nil, fmt.Errorf("create attention_mask tensor: %w", err)
	}
	typeIDs, err := ort.NewTensor(shape, make([]int64, maxSeqLen))
	if err != nil {
		return nil, fmt.Errorf("create token_type_ids tensor: %w", err)
	}
	output, err := ort.NewTensor(outputShape, make([]float32, maxSeqLen*embeddingDim))
	if err != nil {
		return nil, fmt.Errorf("create output tensor: %w", err)
	}

	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		[]ort.ArbitraryTensor{inputIDs, mask, typeIDs},
		[]ort.ArbitraryTensor{output},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create ort session: %w", err)
	}

	return &Embedder{
		tokenizer: tokenizer,
		session:   session,
		inputIDs:  inputIDs,
		mask:      mask,
		typeIDs:   typeIDs,
		output:    output,
	}, nil
}

func (e *Embedder) Embed(text string) ([]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	encoded := e.tokenizer.Encode(text)

	copy(e.inputIDs.GetData(), encoded.InputIDs)
	copy(e.mask.GetData(), encoded.AttentionMask)
	copy(e.typeIDs.GetData(), encoded.TokenTypeIDs)

	if err := e.session.Run(); err != nil {
		return nil, fmt.Errorf("run model: %w", err)
	}

	return meanPool(e.output.GetData(), encoded.AttentionMask), nil
}

func meanPool(data []float32, mask []int64) []float32 {
	result := make([]float32, embeddingDim)

	var count float32
	for i := 0; i < maxSeqLen; i++ {
		if mask[i] == 0 {
			continue
		}
		count++
		for j := 0; j < embeddingDim; j++ {
			result[j] += data[i*embeddingDim+j]
		}
	}

	if count > 0 {
		for j := range result {
			result[j] /= count
		}
	}

	// L2 normalize
	var norm float64
	for _, v := range result {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for j := range result {
			result[j] = float32(float64(result[j]) / norm)
		}
	}

	return result
}

func (e *Embedder) Close() {
	e.session.Destroy()
	e.inputIDs.Destroy()
	e.mask.Destroy()
	e.typeIDs.Destroy()
	e.output.Destroy()
	ort.DestroyEnvironment()
}
