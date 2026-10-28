package main

import (
	"log"
	"net/http"
	_ "time/tzdata" // embeds time zone data, so Europe/Warsaw works on Windows without Go installed

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	db, err := gorm.Open(sqlite.Open("data.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("error opening database: %v", err)
	}

	err = db.AutoMigrate(&Intention{})
	if err != nil {
		log.Fatalf("error migrating database: %v", err)
	}

	router := newRouter(db)

	err = http.ListenAndServe(":8080", router)
	if err != nil {
		log.Fatal("Error starting server: ", err)
	}
}
