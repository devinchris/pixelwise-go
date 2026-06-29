package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
)

// Model holds the classifier weights in fixed-size arrays.
// Fixed sizes ([9], [784]) let the compiler verify dimensions at build time
// and keep the data contiguous in memory — better cache behaviour than slices.
type Model struct {
	classes   [9]string
	coef      [9][784]float64
	intercept [9]float64
}

// weightsJSON is the intermediate type for JSON decoding
type weightsJSON struct {
	Classes   []string    `json:"classes"`
	Coef      [][]float64 `json:"coef"`
	Intercept []float64   `json:"intercept"`
}

func loadModel(path string) (*Model, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read weights: %w", err)
	}

	var w weightsJSON
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, fmt.Errorf("parse weights: %w", err)
	}

	// Validate dimensions before copying — a shape mismatch here means
	// export_weights.py produced unexpected output, not a Go bug.
	if len(w.Classes) != 9 {
		return nil, fmt.Errorf("expected 9 classes, got %d", len(w.Classes))
	}
	if len(w.Coef) != 9 {
		return nil, fmt.Errorf("expected 9 coef rows, got %d", len(w.Coef))
	}
	if len(w.Intercept) != 9 {
		return nil, fmt.Errorf("expected 9 intercept values, got %d", len(w.Intercept))
	}

	var m Model
	for k := 0; k < 9; k++ {
		m.classes[k] = w.Classes[k]
		if len(w.Coef[k]) != 784 {
			return nil, fmt.Errorf("coef row %d: expected 784 features, got %d", k, len(w.Coef[k]))
		}
		// copy() from slice into array slice — idiomatic Go for this pattern.
		copy(m.coef[k][:], w.Coef[k])
		m.intercept[k] = w.Intercept[k]
	}

	return &m, nil
}

func (m *Model) Predict(pixels [][]int) (*ClassifyResponse, error) {
	if len(pixels) != 28 {
		return nil, fmt.Errorf("expected 28 rows, got %d", len(pixels))
	}

	// Binarize and flatten: pixels > 128 -> 1.0, else -> 0.0.
	// Matches Python: (images > 128).astype(float).reshape(-1, 784)
	// Go zero-initialises var declarations, so values <= 128 stay 0.0 implicitly.
	var x [784]float64
	for r, row := range pixels {
		if len(row) != 28 {
			return nil, fmt.Errorf("row %d: expected 28 cols, got %d", r, len(row))
		}
		for c, val := range row {
			if val > 128 {
				x[r*28+c] = 1.0
			}
		}
	}

	// Logits = W*x + b
	var logits [9]float64
	for k := 0; k < 9; k++ {
		s := m.intercept[k]
		for j := 0; j < 784; j++ {
			s += m.coef[k][j] * x[j]
		}
		logits[k] = s
	}

	// Numerically stable softmax: subtract max before exp.
	// Without this, large logits cause exp() to overflow to infinity (and beyond).

	maxLogit := logits[0]
	for _, v := range logits[1:] {
		if v > maxLogit {
			maxLogit = v
		}
	}
	var sumExp float64
	var expLogits [9]float64
	for k, v := range logits {
		expLogits[k] = math.Exp(v - maxLogit)
		sumExp += expLogits[k]
	}
	var probs [9]float64
	for k := range probs {
		probs[k] = expLogits[k] / sumExp
	}

	// Argmax: index of the highest probability = predicted class.
	argmax := 0
	for k := 1; k < 9; k++ {
		if probs[k] > probs[argmax] {
			argmax = k
		}
	}

	scores := make(map[string]float64, 9)
	for k := 0; k < 9; k++ {
		scores[m.classes[k]] = probs[k]
	}

	return &ClassifyResponse{
		Prediction: m.classes[argmax],
		Confidence: probs[argmax],
		Scores:     scores,
	}, nil
}
