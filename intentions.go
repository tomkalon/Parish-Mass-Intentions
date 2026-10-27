package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

type Intention struct {
	ID              int    `json:"id"`
	MassAt          string `json:"mass_at"`
	Content         string `json:"content"`
	IsOutsideParish bool   `json:"is_outside_parish"`
}

var intentions = []Intention{
	{
		ID:              1,
		MassAt:          "2023-09-15",
		Content:         "Intention content",
		IsOutsideParish: true,
	},
	{
		ID:              2,
		MassAt:          "2023-09-16",
		Content:         "Another intention content",
		IsOutsideParish: false,
	},
	{
		ID:              3,
		MassAt:          "2023-09-17",
		Content:         "Yet another intention content",
		IsOutsideParish: true,
	},
}

type intentionHandler struct {
	db *gorm.DB
}

func (h *intentionHandler) list(w http.ResponseWriter, r *http.Request) {
	var intentions []Intention

	err := h.db.Find(&intentions).Error
	if err != nil {
		log.Printf("error finding intentions: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(intentions)
	if err != nil {
		log.Printf("error encoding intentions: %v", err)
	}
}

func (h *intentionHandler) show(w http.ResponseWriter, r *http.Request) {
	var intention Intention

	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)

		return
	}

	err = h.db.First(&intention, id).Error
	if err != nil {
		log.Printf("error finding intention: %v", err)
		http.Error(w, "not found", http.StatusNotFound)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(intention)
}
