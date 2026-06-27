package main

type Model struct {
	coef      [9][784]float64 // weights.json
	intercept [9]float64
}

func loadModel(path string) (*Model, error) {
	// TODO: JSON LESEN, coef und intercept importieren
	return &Model{}, nil
}

func (m *Model) Predict(pixels [][]int) (*ClassifyResponse, error) {
	// TODO: richtige prediction berechnen
	// TODO: mit make_golden verifizieren
	return &ClassifyResponse{
		Prediction: "1",
		Confidence: 1.0,
		Scores:     map[string]float64{"1": 1.0},
	}, nil
}
