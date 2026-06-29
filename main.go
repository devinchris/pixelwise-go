package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// -----------------------------------------------
//	CONFIG
// -----------------------------------------------

type Config struct {
	DBPassword    string
	SecretAPIKey  string
	ListenAddress string // port, default :8000
	useDB         string
}

func loadConfig() Config {
	get := func(key, fallback string) string {
		// Check if config isnt empty
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}

	return Config{
		DBPassword:    get("DB_PASSWORD", ""),
		SecretAPIKey:  get("SECRET_API_KEY", ""),
		ListenAddress: get("LISTEN_ADDR", ":8000"),
		useDB:         get("USE_DB", "false"),
	}
}

// -----------------------------------------------
// Classify Structs
// -----------------------------------------------

// main.py: ClassifyRequest
type ClassifyRequest struct {
	Pixels [][]int
}

// main.py: ClassifyResponse
type ClassifyResponse struct {
	Prediction string
	Confidence float64
	Scores     map[string]float64
}

// classifier is the inference interface used by handleClassify.
type classifier interface {
	Predict(pixels [][]int) (*ClassifyResponse, error)
}

// -----------------------------------------------
// Application state
// -----------------------------------------------

type App struct {
	db     dbStore    // database abstraction (real: pgxStore, test: mockDB)
	model  classifier // inference model (real: *Model, test: mockClassifier)
	apiKey string
	useDB  string
}

// -----------------------------------------------
// Main
// -----------------------------------------------
func main() {
	config := loadConfig()

	//  -- Database pool --
	// Data Source name Mirror the SQLAlchemy python side
	dsn := "postgresql://pixelwise:" + config.DBPassword + "@localhost/pixelwise" +
		"?pool_max_conns=20&pool_min_conns=2"

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("Database could not be opened: %v", err)
	}

	// closes the pool at the end of the main function
	defer pool.Close()

	// Verify DB connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("DB ping failed: %v", err)
	}

	model, err := loadModel("models/weights.json")
	if err != nil {
		log.Fatalf("Cannot load model weights: %v", err)
	}

	app := &App{
		db:     &pgxStore{pool: pool},
		model:  model,
		apiKey: config.SecretAPIKey,
		useDB:  config.useDB,
	}
	f := fiber.New()

	f.Get("/health", app.handleHealth)
	f.Get("/results", app.handleResults)
	f.Post("/classify", app.requireAPIKey, app.handleClassify)

	log.Printf("pixelwise-go listening on %s", config.ListenAddress)
	log.Fatal(f.Listen(config.ListenAddress))
}

// -----------------------------------------------
// Middleware
// -----------------------------------------------

// requireAPIKey checks for a valid API Key in request header
func (a *App) requireAPIKey(c *fiber.Ctx) error {
	if c.Get("X-Api-Key") != a.apiKey {
		return fiber.NewError(fiber.StatusUnauthorized, "Invalid API key")
	}
	return c.Next()
}

// -----------------------------------------------
// Handlers
// -----------------------------------------------
func (a *App) handleHealth(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":        "ok",
		"model_version": "v1",
	})
}

// mirrors the main.py /results endpoint
// returns the 20 most recent predictions from the database
func (a *App) handleResults(c *fiber.Ctx) error {
	rows, err := a.db.queryResults(c.Context())
	if err != nil {
		log.Printf("queryResults failed: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to fetch results")
	}
	return c.JSON(fiber.Map{"results": rows})
}

// mirrors the main.py /classify endpoint
// returns the predicted digit and confidence for the image
func (a *App) handleClassify(c *fiber.Ctx) error {
	var request ClassifyRequest

	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusUnprocessableEntity, "Invalid request body")
	}
	if len(request.Pixels) != 28 || len(request.Pixels[0]) != 28 {
		return fiber.NewError(fiber.StatusUnprocessableEntity, "Image must be 28x28")
	}

	result, err := a.model.Predict(request.Pixels)
	if err != nil {
		log.Printf("Prediction failed: %v", err)
		return fiber.NewError(fiber.StatusInternalServerError, "Prediction failed")
	}

	if a.useDB == "true" {
		if err := a.db.insertPrediction(c.Context(), result.Prediction, result.Confidence); err != nil {
			log.Printf("insertPrediction failed: %v", err)
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to save prediction")
		}
	}

	return c.JSON(result)
}

// -----------------------------------------------
// Error handler
// -----------------------------------------------
func jsonErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Internal Server Error"
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}
	return c.Status(code).JSON(fiber.Map{
		"detail": message,
	})
}
