package schemabuilder

import (
	"github.com/go-playground/validator/v10"
	"sync"
)

var validate *validator.Validate
var once sync.Once

func NewValidate() *validator.Validate {
	once.Do(func() {
		validate = validator.New()
	})
	return validate
}
