package graphql

import (
	"reflect"
	"strings"
)

func GetField(typ reflect.Value, name string) *reflect.Value {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldTyp := typ.Type().Field(i)
		tag := fieldTyp.Tag.Get("graphql")
		if tag == "" || tag == "-" {
			if fieldTyp.Name == name {
				return &field
			}
		}
		split := strings.Split(tag, "|")
		if split[0] == name {
			return &field
		}
	}
	return nil
}
