package main

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

type GoldenCase struct {
	Pixels     [][]int            `json:"pixels"`
	Prediction string             `json:"prediction"`
	Confidence float64            `json:"confidence"`
	Scores     map[string]float64 `json:"scores"`
}

const confidenceTolerance = 1e-6

func TestGolden(t *testing.T) {
	// load model
	loadedModel, err := loadModel("models/weights.json")
	if err != nil {
		t.Fatalf("Failed to load model: %v", err)
	}

	// load golden cases
	rawJSON, err := os.ReadFile("testdata/golden.json")
	if err != nil {
		t.Fatalf("Failed to read golden file: %v", err)
	}
	var goldenCases []GoldenCase
	if err := json.Unmarshal(rawJSON, &goldenCases); err != nil {
		t.Fatalf("Failed to unmarshal golden cases: %v", err)
	}

	// Test each case
	for caseIndex, expectedOutput := range goldenCases {
		actualOutput, err := loadedModel.Predict(expectedOutput.Pixels)
		if err != nil {
			t.Errorf("case %d: Predict() Fehler: %v", caseIndex, err)
			// skip this case, but continue with the next one
			continue
		}

		// prediction must be equal
		// difference means wrong class, critical error
		if actualOutput.Prediction != expectedOutput.Prediction {
			t.Errorf("case %d: Prediction falsch: Go=%q Python=%q",
				caseIndex, actualOutput.Prediction, expectedOutput.Prediction)
		}

		// confidence shall be equal within floating point tolerance
		confidenceDelta := math.Abs(actualOutput.Confidence - expectedOutput.Confidence)
		if confidenceDelta > confidenceTolerance {
			t.Errorf("case %d: Confidence-Abweichung zu groß: |%f - %f| = %e > %e",
				caseIndex, actualOutput.Confidence, expectedOutput.Confidence,
				confidenceDelta, confidenceTolerance)
		}

		// Test each class score on its own
		if len(actualOutput.Scores) != len(expectedOutput.Scores) {
			t.Errorf("case %d: Scores length mismatch: Go=%d Python=%d",
				caseIndex, len(actualOutput.Scores), len(expectedOutput.Scores))
		}
		for className, expectedScore := range expectedOutput.Scores {
			actualScore, exists := actualOutput.Scores[className]
			if !exists {
				t.Errorf("case %d: Klasse %q fehlt in Go-Scores", caseIndex, className)
				continue
			}
			scoreDelta := math.Abs(actualScore - expectedScore)
			if scoreDelta > confidenceTolerance {
				t.Errorf("case %d: Score für Klasse %q: |%f - %f| = %e > %e",
					caseIndex, className, actualScore, expectedScore,
					scoreDelta, confidenceTolerance)
			}
		}
	}
}
