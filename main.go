package main

import (
	"log"

	"github.com/gofiber/fiber/v3"
)

type Config struct {
	DBPassword    string
	ModelPath     string // weights.json
	SecretAPIKey  string
	ListenAddress string // port, default :8000
}

// Dependencies
type ClassifyRequest struct {
	Pixels [][]int
}

func Health(c fiber.Ctx) error {
	return c.SendString("Healthy")
}

func main() {
	app := fiber.New()

	app.Get("/health", Health)

	log.Fatal(app.Listen(":3000"))
}
