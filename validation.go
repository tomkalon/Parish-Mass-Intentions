package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
)

func newValidator() *validator.Validate {
	validate := validator.New()

	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name, _, _ := strings.Cut(fld.Tag.Get("json"), ",")
		return name
	})

	err := validate.RegisterValidation("quarter_hour", isQuarterHour)
	if err != nil {
		panic(err)
	}

	return validate
}

func isQuarterHour(fl validator.FieldLevel) bool {
	t, err := time.Parse(massAtLayout, fl.Field().String())
	if err != nil {
		return false
	}

	return t.Minute()%15 == 0
}

func writeValidationError(w http.ResponseWriter, err error) {
	var validationErrs validator.ValidationErrors
	if !errors.As(err, &validationErrs) {
		log.Printf("unexpected validation error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	errs := make(map[string]string)
	for _, fe := range validationErrs {
		errs[fe.Field()] = validationMessage(fe)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]any{"errors": errs})
}

func validationMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "pole jest wymagane"
	case "datetime":
		return "nieprawidłowy format, oczekiwano RRRR-MM-DD GG:MM"
	case "quarter_hour":
		return "godzina musi być pełnym kwadransem (:00, :15, :30, :45)"
	case "max":
		return fmt.Sprintf("maksymalnie %s znaków", fe.Param())
	default:
		return "nieprawidłowa wartość"
	}
}
