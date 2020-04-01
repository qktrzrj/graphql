package schemabuilder

import "github.com/go-playground/validator/v10"

var validate *validator.Validate

func NewValidate() *validator.Validate {
	validate = validator.New()
	return validate
}
