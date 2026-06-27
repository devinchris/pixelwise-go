package main

import (
	"log"

	"github.com/gofiber/fiber/v3"
)

// -----------------------------------------------
//	CONFIG 
// -----------------------------------------------

type Config struct {
	DBPassword    string
	ModelPath     string // weights.json
	SecretAPIKey  string
	ListenAddress string // port, default :8000
}

func loadConfig() Config {
	get := func(key, fallback string) string {
		// Check if config isnt empty
		if v:= os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}

	return Config{
		DBPassword:   get("DB_PASSWORD", ""),
		SecretAPIKey: get("SECRET_API_KEY", ""),
		ModelPath:    get("MODEL_PATH", "models/weights.json"),
		ListenAddr:   get("LISTEN_ADDR", ":8000"),
	}
}




// Dependencies
type ClassifyRequest struct {
	Pixels [][]int
}

func Health(c fiber.Ctx) error {
	return c.SendString("Healthy")
}

func main() {
	config := loadConfig()
	app := fiber.New()

	app.Get("/health", Health)

	log.Fatal(app.Listen(":3000"))
}
