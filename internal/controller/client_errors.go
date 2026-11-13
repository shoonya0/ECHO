package controller

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// Errors sent to clients never carry err.Error() (#41): it can hold driver
// errors, hosts, or Go type and struct names. Log the full error on the server
// and send a fixed text, or one built from a known error as below.

// invalidBodyDetail is the detail for a body that isn't valid JSON.
const invalidBodyDetail = "request body is not valid JSON for this endpoint"

func init() {
	// Report validation failures by their JSON field names, which clients know.
	// Every request body is bound in this package, so this is set before any bind.
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
// field that failed validation with its rule ("password: min=8"), a field of
// the wrong type ("age: wrong type"), or invalidBodyDetail. Fields are named by
// their JSON path, never by Go struct or type names.
func bindErrorDetail(err error) string {
	var fieldErrs validator.ValidationErrors
	if errors.As(err, &fieldErrs) {
		parts := make([]string, 0, len(fieldErrs))
		for _, fe := range fieldErrs {
			rule := fe.Tag()
			if fe.Param() != "" {
				rule += "=" + fe.Param()
			}
			parts = append(parts, fieldPath(fe.Namespace())+": "+rule)
		}
		return strings.Join(parts, "; ")
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) && typeErr.Field != "" {
		return typeErr.Field + ": wrong type"
	}
	return invalidBodyDetail
}

// fieldPath drops the root struct's Go name from a validator namespace:
// "signupRequest.settings.name" becomes "settings.name".
func fieldPath(namespace string) string {
	if _, rest, ok := strings.Cut(namespace, "."); ok {
		return rest
	}
	return namespace
}
