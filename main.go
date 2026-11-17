package main

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"go-parafia/internal/backup"
	"go-parafia/internal/database"
	"go-parafia/internal/update"
	"go-parafia/internal/web"
)

func main() {
	waitForPreviousVersion()

	dir, err := prepareDataDir()
	if err != nil {
		log.Fatalf("error preparing data directory: %v", err)
	}

	db, err := database.Open(filepath.Join(dir, "data.db"))
	if err != nil {
		log.Fatal(err)
	}

	warning := ""
	err = database.Check(db)
	if err != nil {
		log.Printf("database check failed: %v", err)
		warning = "Sprawdzenie bazy danych wykryło błąd. Dane mogą być niekompletne – przywróć je z kopii w Ustawieniach (Import danych)."
	}

	backups, err := backup.Load(db, filepath.Join(dir, "kopia.json"))
	if err != nil {
		log.Fatalf("error loading backup settings: %v", err)
	}

	updater := update.New(appVersion(winres), programPath(), restartProgram())
	if updater.CanInstall() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			err := updater.CheckQuietly(ctx)
			if err != nil {
				log.Printf("automatic update check failed: %v", err)
			}
		}()
	}

	opts := web.Options{
		DataDir:         dir,
		SaveFile:        saveFileDialog(),
		Backups:         backups,
		ChooseDir:       chooseDirDialog(),
		DatabaseWarning: warning,
		Version:         appVersion(winres),
		Updater:         updater,
	}

	err = run(newRouter(db, opts), dir, backups)
	if err != nil {
		log.Fatalf("error running application: %v", err)
	}
}
