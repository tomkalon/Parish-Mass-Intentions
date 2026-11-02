package main

import (
	"log"
	"path/filepath"

	"go-parafia/internal/backup"
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

	backups, err := backup.Load(db, filepath.Join(dir, "kopia.json"))
	if err != nil {
		log.Fatalf("error loading backup settings: %v", err)
	}

	opts := web.Options{
		DataDir:   dir,
		SaveFile:  saveFileDialog(),
		Backups:   backups,
		ChooseDir: chooseDirDialog(),
	}

	err = run(newRouter(db, opts), dir, backups)
	if err != nil {
		log.Fatalf("error running application: %v", err)
	}
}
