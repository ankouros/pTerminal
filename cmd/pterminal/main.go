package main

import (
	"log"

	"github.com/ankouros/pterminal/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
