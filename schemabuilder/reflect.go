package schemabuilder

import (
	"reflect"
	"strings"
)

func getField(source interface{}, name string) reflect.Type {
	typ := reflect.TypeOf(source)
	if field, ok := typ.FieldByName(name); ok {
		return field.Type
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("graphql")
		if tag == "" || tag == "-" {
			continue
		}
		split := strings.Split(tag, ",")
		if split[0] == name {
			return field.Type
		}
	}
	return nil
}

func getMethod(source interface{}, name string) reflect.Type {
	typ := reflect.TypeOf(source)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	if field, ok := typ.MethodByName(name); ok {
		return field.Type
	}
	return nil
}

func GetField(source reflect.Value, name string) *reflect.Value {
	typ := reflect.ValueOf(source)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldTyp := typ.Type().Field(i)
		tag := fieldTyp.Tag.Get("graphql")
		if tag == "" || tag == "-" {
			if fieldTyp.Name == name {
				return &field
			}
		}
		split := strings.Split(tag, ",")
		if split[0] == name {
			return &field
		}
	}
	return nil
}
