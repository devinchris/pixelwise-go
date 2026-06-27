package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PredictionRow struct {
	ID           int       `json:"id"`
	Prediction   string    `json:"prediction"`
	Confidence   float64   `json:"confidence"`
	ModelVersion string    `json:"model_version"`
	CreatedAt    time.Time `json:"created_at"`
}

func queryResults(ctx context.Context, pool *pgxpool.Pool) ([]PredictionRow, error) {
	// TODO: SELECT id, prediction, confidence, model_version, created_at
	//       FROM predictions ORDER BY created_at DESC LIMIT 20
	return []PredictionRow{}, nil
}

func insertPrediction(ctx context.Context, pool *pgxpool.Pool, prediction string, confidence float64) error {
	// TODO: INSERT INTO predictions (prediction, confidence, model_version)
	//       VALUES ($1, $2, 'v1')
	return nil
}
