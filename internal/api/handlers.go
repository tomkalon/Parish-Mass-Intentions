package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"

	"go-parafia/internal/intentions"
)

type intentionHandler struct {
	db       *gorm.DB
	validate *validator.Validate
}

func RegisterRoutes(router chi.Router, db *gorm.DB) {
	h := &intentionHandler{db: db, validate: intentions.NewValidator()}

	router.Get("/intentions", h.list)
	router.Get("/intentions/{id}", h.show)
	router.Put("/intentions/{id}", h.put)
	router.Patch("/intentions/{id}", h.patch)
	router.Delete("/intentions/{id}", h.delete)
	router.Post("/intentions", h.create)
}

func (h *intentionHandler) list(w http.ResponseWriter, r *http.Request) {
	var all []intentions.Intention

	err := h.db.Find(&all).Error
	if err != nil {
		log.Printf("error finding intentions: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)

		return
	}

	writeJSON(w, http.StatusOK, all)
}

func (h *intentionHandler) findIntention(w http.ResponseWriter, r *http.Request) (intentions.Intention, bool) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return intentions.Intention{}, false
	}

	var intention intentions.Intention
	err = h.db.First(&intention, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("intention not found: %d", id)
		http.Error(w, "intention not found", http.StatusNotFound)
		return intentions.Intention{}, false
	}
	if err != nil {
		log.Printf("error finding intention: %d, %v", id, err)
		http.Error(w, "error finding intention", http.StatusInternalServerError)
		return intentions.Intention{}, false
	}

	return intention, true
}

func (h *intentionHandler) show(w http.ResponseWriter, r *http.Request) {
	intention, ok := h.findIntention(w, r)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, intention)
}

func (h *intentionHandler) put(w http.ResponseWriter, r *http.Request) {
	intention, ok := h.findIntention(w, r)
	if !ok {
		return
	}

	var input intentions.Input
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

	intention.Apply(input)

	err = h.db.Save(&intention).Error
	if err != nil {
		log.Printf("error saving intention: %d, %v", intention.ID, err)
		http.Error(w, "error saving intention", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, intention)
}

func (h *intentionHandler) patch(w http.ResponseWriter, r *http.Request) {
	intention, ok := h.findIntention(w, r)
	if !ok {
		return
	}

	input := intentions.Input{
		MassAt:          intention.MassAt.In(intentions.TimeZone).Format(intentions.MassAtLayout),
		Content:         intention.Content,
		IsOutsideParish: intention.IsOutsideParish,
		IsGregorian:     intention.IsGregorian,
		IsDeceased:      intention.IsDeceased,
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

	intention.Apply(input)

	err = h.db.Save(&intention).Error
	if err != nil {
		log.Printf("error saving intention: %d, %v", intention.ID, err)
		http.Error(w, "error saving intention", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, intention)
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
	var input intentions.Input
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

	var intention intentions.Intention
	intention.Apply(input)

	err = h.db.Create(&intention).Error
	if err != nil {
		log.Printf("error creating intention: %v", err)
		http.Error(w, "error creating intention", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, intention)
}
