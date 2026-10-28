package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"
)

const massAtLayout = "2006-01-02 15:04"

var parishLocation = mustLoadLocation("Europe/Warsaw")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

type Intention struct {
	ID              int       `json:"id"`
	MassAt          time.Time `json:"mass_at"`
	Content         string    `json:"content"`
	IsOutsideParish bool      `json:"is_outside_parish"`
}

type IntentionInput struct {
	MassAt          string `json:"mass_at" validate:"required,datetime=2006-01-02 15:04,quarter_hour"`
	Content         string `json:"content" validate:"required,max=500"`
	IsOutsideParish bool   `json:"is_outside_parish"`
}

func (i *Intention) apply(input IntentionInput) {
	i.MassAt, _ = time.ParseInLocation(massAtLayout, input.MassAt, parishLocation)
	i.Content = input.Content
	i.IsOutsideParish = input.IsOutsideParish
}

type intentionHandler struct {
	db       *gorm.DB
	validate *validator.Validate
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

func (h *intentionHandler) findIntention(w http.ResponseWriter, r *http.Request) (Intention, bool) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return Intention{}, false
	}

	var intention Intention
	err = h.db.First(&intention, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("intention not found: %d", id)
		http.Error(w, "intention not found", http.StatusNotFound)
		return Intention{}, false
	}
	if err != nil {
		log.Printf("error finding intention: %d, %v", id, err)
		http.Error(w, "error finding intention", http.StatusInternalServerError)
		return Intention{}, false
	}

	return intention, true
}

func (h *intentionHandler) show(w http.ResponseWriter, r *http.Request) {
	intention, ok := h.findIntention(w, r)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(intention)
}

func (h *intentionHandler) put(w http.ResponseWriter, r *http.Request) {
	intention, ok := h.findIntention(w, r)
	if !ok {
		return
	}

	var input IntentionInput
	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	input.Content = strings.TrimSpace(input.Content)
	err = h.validate.Struct(input)
	if err != nil {
		writeValidationError(w, err)
		return
	}

	intention.apply(input)

	err = h.db.Save(&intention).Error
	if err != nil {
		log.Printf("error saving intention: %d, %v", intention.ID, err)
		http.Error(w, "error saving intention", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(intention)
}

func (h *intentionHandler) patch(w http.ResponseWriter, r *http.Request) {
	intention, ok := h.findIntention(w, r)
	if !ok {
		return
	}

	input := IntentionInput{
		MassAt:          intention.MassAt.In(parishLocation).Format(massAtLayout),
		Content:         intention.Content,
		IsOutsideParish: intention.IsOutsideParish,
	}

	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	input.Content = strings.TrimSpace(input.Content)
	err = h.validate.Struct(input)
	if err != nil {
		writeValidationError(w, err)
		return
	}

	intention.apply(input)

	err = h.db.Save(&intention).Error
	if err != nil {
		log.Printf("error saving intention: %d, %v", intention.ID, err)
		http.Error(w, "error saving intention", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(intention)
}

func (h *intentionHandler) delete(w http.ResponseWriter, r *http.Request) {
	intention, ok := h.findIntention(w, r)
	if !ok {
		return
	}

	err := h.db.Delete(&intention).Error
	if err != nil {
		log.Printf("error deleting intention: %d, %v", intention.ID, err)
		http.Error(w, "error deleting intention", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *intentionHandler) create(w http.ResponseWriter, r *http.Request) {
	var input IntentionInput
	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	input.Content = strings.TrimSpace(input.Content)
	err = h.validate.Struct(input)
	if err != nil {
		writeValidationError(w, err)
		return
	}

	var intention Intention
	intention.apply(input)

	err = h.db.Create(&intention).Error
	if err != nil {
		log.Printf("error creating intention: %v", err)
		http.Error(w, "error creating intention", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(intention)
}
