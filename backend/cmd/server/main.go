// Package main is the help-my-run backend server entrypoint.
// This is a scaffolding stub; the real server is implemented by the SERVER tasks.
package main

import (
	"fmt"

	// Blank imports pin the core dependencies so `go mod tidy` keeps them
	// recorded at their contract-specified versions until the real server
	// code (SERVER tasks) imports them directly.
	_ "github.com/go-chi/chi/v5"
	_ "github.com/joho/godotenv"
	_ "github.com/kelseyhightower/envconfig"
	_ "github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func main() {
	fmt.Println("help-my-run backend (scaffold stub)")
}
