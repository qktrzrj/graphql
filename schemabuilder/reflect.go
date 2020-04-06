package schemabuilder

import (
	"context"
	"fmt"
	"go/ast"
	"reflect"
	"strings"
)

type structFields struct {
	list      []*reflect.Value
	nameIndex map[string]int
}

func parseFieldTag(field reflect.StructField) (skip, nonnull bool, name, desc string) {
	if !ast.IsExported(field.Name) {
		skip = true
		return
	}
	tag := field.Tag.Get("graphql")
	if tag == "" {
		name = field.Name
		return
	}
	if tag == "-" {
		skip = true
		return
	}
	split := strings.Split(tag, ";")
	name = split[0]
	if len(split) > 1 {
		desc = split[1]
	}
	if len(split) > 2 {
		nonnull = split[2] == "nonnull"
	}
	return
}

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
		split := strings.Split(tag, ";")
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
		split := strings.Split(tag, ";")
		if split[0] == name {
			return &field
		}
	}
	return nil
}

func Convert(args map[string]interface{}, typ reflect.Type) (interface{}, error) {
	fields := structFields{
		nameIndex: make(map[string]int),
	}
	var typPtr bool
	if typ.Kind() == reflect.Ptr {
		typPtr = true
		typ = typ.Elem()
	}
	tv := reflect.New(typ).Elem()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("graphql")
		if tag == "-" || !ast.IsExported(field.Name) {
			continue
		}
		tf := tv.Field(i)
		fields.list = append(fields.list, &tf)
		if tag == "" {
			fields.nameIndex[field.Name] = len(fields.list) - 1
			continue
		}
		split := strings.Split(tag, ";")
		fields.nameIndex[split[0]] = len(fields.list) - 1
	}

	for k, v := range args {
		if v == nil {
			continue
		}
		var f reflect.Value
		if i, ok := fields.nameIndex[k]; ok {
			f = *fields.list[i]
		} else {
			continue
		}
		vv := reflect.ValueOf(v)
		if err := value(f, vv); err != nil {
			return nil, err
		}
	}
	if typPtr {
		return tv.Addr().Interface(), nil
	}
	return tv.Interface(), nil
}

func value(f reflect.Value, v reflect.Value) error {
	if f.IsValid() {
		f.Set(reflect.New(f.Type()).Elem())
	}
	for v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	for f.Kind() == reflect.Ptr {
		if f.IsNil() {
			f.Set(reflect.New(f.Type().Elem()))
		}
		f = f.Elem()
	}
	if f.Kind() == reflect.Slice {
		if v.Kind() != reflect.Slice {
			return fmt.Errorf("field %s type is slice, but value %s not", f.Type().String(), v.Type().String())
		}
		fs := reflect.MakeSlice(f.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			if err := value(fs.Index(i), v.Index(i)); err != nil {
				return err
			}
		}
		f.Set(fs)
		return nil
	}
	if f.Kind() == reflect.Map {
		if v.Kind() != reflect.Map {
			return fmt.Errorf("field %s type is map, but value %s not", f.Type().String(), v.Type().String())
		}
		fm := reflect.MakeMapWithSize(f.Type(), v.Len())
		for _, k := range v.MapKeys() {
			fv := reflect.New(f.Type().Elem()).Elem()
			if err := value(fv, v.MapIndex(k)); err != nil {
				return err
			}
			fm.SetMapIndex(k, fv)
		}
		f.Set(fm)
		return nil
	}
	if !v.Type().ConvertibleTo(f.Type()) {
		return nil
	}
	f.Set(v.Convert(f.Type()))
	return nil
}

// Common Types that we will need to perform type assertions against.
var errType = reflect.TypeOf((*error)(nil)).Elem()
var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()
