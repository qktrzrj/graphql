package schemabuilder

import (
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"sync"
)

var (
	validate   *validator.Validate
	once       sync.Once
	translator ut.Translator
)

func NewValidate() *validator.Validate {
	once.Do(func() {
		validate = validator.New()
	})
	return validate
}

func NewTranslator(t ut.Translator) {
	translator = t
}
