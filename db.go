package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// mirrors the Python prediction SQLAlchemy model (app/models.py).
// json tags must match the field names returned by the /results endpoint.
type PredictionRow struct {
	ID           int       `json:"id"`
	Prediction   string    `json:"prediction"`
	Confidence   float64   `json:"confidence"`
	ModelVersion string    `json:"model_version"`
	CreatedAt    time.Time `json:"created_at"`
}

// mirrors the Python /results handler:
// db.query(Prediction).order_by(Prediction.created_at.desc()).limit(20).all()
func queryResults(ctx context.Context, pool *pgxpool.Pool) ([]PredictionRow, error) {
	const query = `
		SELECT id, prediction, confidence, model_version, created_at
		FROM predictions
		ORDER BY created_at DESC
		LIMIT 20`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("queryResults: %w", err)
	}
	defer rows.Close()

	// Pre-allocate with a reasonable capacity to avoid repeated allocations.
	results := make([]PredictionRow, 0, 20)

	for rows.Next() {
		var row PredictionRow
		if err := rows.Scan(
			&row.ID,
			&row.Prediction,
			&row.Confidence,
			&row.ModelVersion,
			&row.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("queryResults scan: %w", err)
		}
		results = append(results, row)
	}

	// rows.Err() captures errors that occurred during iteration.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("queryResults rows: %w", err)
	}

	return results, nil
}

// insertPrediction writes a single classification result to the database.
// model_version is always 'v1', matching the Python side

// synchronous commit is used here exactly like the Python side
// The bottleneck here is intentional and will probably be significant
func insertPrediction(ctx context.Context, pool *pgxpool.Pool, prediction string, confidence float64) error {
	const query = `
		INSERT INTO predictions (prediction, confidence, model_version)
		VALUES ($1, $2, 'v1')`

	_, err := pool.Exec(ctx, query, prediction, confidence)
	if err != nil {
		return fmt.Errorf("insertPrediction: %w", err)
	}

	return nil
}
