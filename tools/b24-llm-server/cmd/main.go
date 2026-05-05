package main

import (
	"log"

	"github.com/magomedcoder/gen/tools/b24-llm-server/internal"
)

func main() {
	app, err := internal.New()
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
