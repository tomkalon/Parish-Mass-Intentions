package intentions

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
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
	err = validate.RegisterValidation("amount", isAmount)
	if err != nil {
		panic(err)
	}

	return validate
}

func isAmount(fl validator.FieldLevel) bool {
	amount := fl.Field().Int()

	return amount >= 0 && amount <= int64(MaxAmount)
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

const RequiredMessage = "Pole jest wymagane."

func validationMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return RequiredMessage
	case "datetime":
		return "Nieprawidłowy format, oczekiwano RRRR-MM-DD GG:MM."
	case "quarter_hour":
		return "Godzina musi być pełnym kwadransem (:00, :15, :30, :45)."
	case "max":
		return fmt.Sprintf("Maksymalnie %s znaków.", fe.Param())
	case "amount":
		return "Kwota musi wynosić od 0 do " + MaxAmount.String() + " (w groszach: od 0 do " + strconv.FormatInt(int64(MaxAmount), 10) + ")."
	default:
		return "Nieprawidłowa wartość."
	}
}
