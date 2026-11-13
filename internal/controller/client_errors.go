package controller

import (
	"errors"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// Errors sent to clients never carry err.Error() (#41): it can hold driver
// errors, hosts, or Go type and struct names. Log the full error on the server
// and send a fixed text, or one built from a known error as below.

// invalidBodyDetail is the detail for a body that isn't JSON, or has a field of
// the wrong type.
const invalidBodyDetail = "request body is not valid JSON for this endpoint"

func init() {
	// Report validation failures by their JSON field names, which clients know.
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterTagNameFunc(func(field reflect.StructField) string {
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "-" {
				return ""
			}
			if name == "" {
				return field.Name
			}
			return name
		})
	}
}

// bindErrorDetail describes a request-body bind error for the client: each
// field that failed validation with its rule ("email: required"), or
// invalidBodyDetail for anything else.
func bindErrorDetail(err error) string {
	var fieldErrs validator.ValidationErrors
	if !errors.As(err, &fieldErrs) {
		return invalidBodyDetail
	}
	parts := make([]string, 0, len(fieldErrs))
	for _, fe := range fieldErrs {
		parts = append(parts, fe.Field()+": "+fe.Tag())
	}
	return strings.Join(parts, "; ")
}
