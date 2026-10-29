package intentions

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
)

func NewValidator() *validator.Validate {
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
	t, err := time.Parse(MassAtLayout, fl.Field().String())
	if err != nil {
		return false
	}

	return t.Minute()%15 == 0
}

func ValidationMessages(err error) (map[string]string, bool) {
	var validationErrs validator.ValidationErrors
	if !errors.As(err, &validationErrs) {
		return nil, false
	}

	messages := make(map[string]string)
	for _, fe := range validationErrs {
		messages[fe.Field()] = validationMessage(fe)
	}

	return messages, true
}

func validationMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "Pole jest wymagane."
	case "datetime":
		return "Nieprawidłowy format, oczekiwano RRRR-MM-DD GG:MM."
	case "quarter_hour":
		return "Godzina musi być pełnym kwadransem (:00, :15, :30, :45)."
	case "max":
		return fmt.Sprintf("Maksymalnie %s znaków.", fe.Param())
	default:
		return "Nieprawidłowa wartość."
	}
}
