package main

import (
	"log"
	"path/filepath"

	"go-parafia/internal/database"
	"go-parafia/internal/web"
)

func main() {
	dir, err := prepareDataDir()
	if err != nil {
		log.Fatalf("error preparing data directory: %v", err)
	}

	db, err := database.Open(filepath.Join(dir, "data.db"))
	if err != nil {
		log.Fatal(err)
	}

	err = run(newRouter(db, web.Options{DataDir: dir, SaveFile: saveFileDialog()}), dir)
	if err != nil {
		log.Fatalf("error running application: %v", err)
	}
}
