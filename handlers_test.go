package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// -----------------------------------------------
// Mocks
// -----------------------------------------------

// mockDB implements dbStore so handlers can be tested without a real database.
// Both fields are set per-test: rows for a success case, err for an error case.
type mockDB struct {
	rows []PredictionRow
	err  error
}

func (m *mockDB) queryResults(_ context.Context) ([]PredictionRow, error) {
	return m.rows, m.err
}

func (m *mockDB) insertPrediction(_ context.Context, _ string, _ float64) error {
	return m.err
}

// mockClassifier implements classifier and returns a fixed response,
// so handleClassify tests do not need models/weights.json.
type mockClassifier struct {
	response *ClassifyResponse
}

func (m *mockClassifier) Predict(_ [][]int) (*ClassifyResponse, error) {
	return m.response, nil
}

// -----------------------------------------------
// Test helper
// -----------------------------------------------

// newTestFiber creates a minimal Fiber app wired with the three routes
// and the custom error handler, using the provided (mock) dependencies.
func newTestFiber(db dbStore, model classifier, apiKey string, useDB string) *fiber.App {
	a := &App{db: db, model: model, apiKey: apiKey, useDB: useDB}
	f := fiber.New(fiber.Config{ErrorHandler: jsonErrorHandler})
	f.Get("/health", a.handleHealth)
	f.Get("/results", a.handleResults)
	f.Post("/classify", a.requireAPIKey, a.handleClassify)
	return f
}

// make28x28 returns a 28×28 int grid where every pixel is the given value.
func make28x28(value int) [][]int {
	grid := make([][]int, 28)
	for i := range grid {
		row := make([]int, 28)
		for j := range row {
			row[j] = value
		}
		grid[i] = row
	}
	return grid
}

// -----------------------------------------------
// Tests: handleHealth
// -----------------------------------------------

func TestHandleHealth(t *testing.T) {
	f := newTestFiber(nil, nil, "", "false")

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := f.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// Must return 200 OK
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}

	// Must return exactly the expected JSON body
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status field: got %q, want %q", body["status"], "ok")
	}
	if body["model_version"] != "v1" {
		t.Errorf("model_version field: got %q, want %q", body["model_version"], "v1")
	}
}

// -----------------------------------------------
// Tests: handleClassify — authentication
// -----------------------------------------------

func TestHandleClassify_MissingAPIKey(t *testing.T) {
	f := newTestFiber(nil, nil, "secret", "false")

	// No X-Api-Key header at all
	req := httptest.NewRequest("POST", "/classify", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != 401 {
		t.Errorf("status: got %d, want 401", resp.StatusCode)
	}

	// Error body must match FastAPI shape: {"detail": "..."}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["detail"] != "Invalid API key" {
		t.Errorf("detail: got %q, want %q", body["detail"], "Invalid API key")
	}
}

func TestHandleClassify_WrongAPIKey(t *testing.T) {
	f := newTestFiber(nil, nil, "correct-key", "false")

	req := httptest.NewRequest("POST", "/classify", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "wrong-key")
	resp, err := f.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != 401 {
		t.Errorf("status: got %d, want 401", resp.StatusCode)
	}
}

// -----------------------------------------------
// Tests: handleClassify — input validation
// -----------------------------------------------

func TestHandleClassify_InvalidBody(t *testing.T) {
	f := newTestFiber(nil, nil, "key", "false")

	req := httptest.NewRequest("POST", "/classify", strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "key")
	resp, err := f.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != 422 {
		t.Errorf("status: got %d, want 422", resp.StatusCode)
	}
}

func TestHandleClassify_WrongDimensions(t *testing.T) {
	f := newTestFiber(nil, nil, "key", "false")

	// Send a 3×3 pixel grid instead of 28×28
	payload, _ := json.Marshal(map[string]any{
		"pixels": [][]int{{0, 1, 2}, {3, 4, 5}, {6, 7, 8}},
	})
	req := httptest.NewRequest("POST", "/classify", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "key")
	resp, err := f.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != 422 {
		t.Errorf("status: got %d, want 422", resp.StatusCode)
	}
}

// -----------------------------------------------
// Tests: handleClassify — success path
// -----------------------------------------------

func TestHandleClassify_Success(t *testing.T) {
	// Prepare a deterministic mock classifier response
	fakeResponse := &ClassifyResponse{
		Prediction: "7",
		Confidence: 0.987,
		Scores:     map[string]float64{"1": 0.001, "7": 0.987, "9": 0.012},
	}
	f := newTestFiber(&mockDB{}, &mockClassifier{response: fakeResponse}, "key", "false")

	payload, _ := json.Marshal(map[string]any{"pixels": make28x28(200)})
	req := httptest.NewRequest("POST", "/classify", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "key")

	resp, err := f.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}

	// Decode and verify the response fields
	var body ClassifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Prediction != "7" {
		t.Errorf("prediction: got %q, want %q", body.Prediction, "7")
	}
	if body.Confidence != 0.987 {
		t.Errorf("confidence: got %f, want %f", body.Confidence, 0.987)
	}
}

// -----------------------------------------------
// Tests: handleResults
// -----------------------------------------------

func TestHandleResults_Success(t *testing.T) {
	// Prepare two fake rows as the mock DB would return
	fakeRows := []PredictionRow{
		{ID: 2, Prediction: "5", Confidence: 0.91, ModelVersion: "v1", CreatedAt: time.Now()},
		{ID: 1, Prediction: "3", Confidence: 0.83, ModelVersion: "v1", CreatedAt: time.Now()},
	}
	f := newTestFiber(&mockDB{rows: fakeRows}, nil, "", "false")

	req := httptest.NewRequest("GET", "/results", nil)
	resp, err := f.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}

	// Response must be wrapped in {"results": [...]}
	var body struct {
		Results []PredictionRow `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Results) != 2 {
		t.Errorf("results length: got %d, want 2", len(body.Results))
	}
	if body.Results[0].Prediction != "5" {
		t.Errorf("first prediction: got %q, want %q", body.Results[0].Prediction, "5")
	}
}

func TestHandleResults_DBError(t *testing.T) {
	// mockDB returns an error to simulate a database failure
	f := newTestFiber(&mockDB{err: errors.New("connection refused")}, nil, "", "false")

	req := httptest.NewRequest("GET", "/results", nil)
	resp, err := f.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// Handler must return 500 and not panic
	if resp.StatusCode != 500 {
		t.Errorf("status: got %d, want 500", resp.StatusCode)
	}
}
