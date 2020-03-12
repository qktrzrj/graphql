package schemabuilder

import (
	"fmt"
	"github.com/unrotten/graphql/internal"
	"reflect"
	"sort"
)

// buildStruct is a function for building the graphql.Type for a passed in struct type.
// This function reads the "Object" information and Fields of the passed in struct to create a "graphql.Object" type
// that represents this type and can be used to resolve graphql requests.
func (sb *schemaBuilder) buildStruct(typ reflect.Type) error {
	if sb.types[typ] != nil {
		return nil
	}

	var name string
	var description string
	var resolves map[string]*fieldResolve
	var interfaces []*Interface
	if object, ok := sb.objects[typ]; ok {
		name = object.Name
		description = object.Desc
		resolves = object.FieldResolve
		interfaces = object.Interface
	} else {
		if typ.Name() != "query" && typ.Name() != "mutation" && typ.Name() != "Subscription" {
			return fmt.Errorf("%s not registered as object", typ.Name())
		}
	}

	if name == "" {
		name = typ.Name()
		if name == "" {
			return fmt.Errorf("bad type %s: should have a name", typ)
		}
	}

	object := &internal.Object{
		Name:       name,
		Desc:       description,
		Fields:     make(map[string]*internal.Field),
		Interfaces: make(map[string]*internal.Interface),
	}
	sb.types[typ] = object

	var names []string
	for name := range resolves {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		resolve := resolves[name]

		built, err := sb.buildFunction(typ, resolve)
		if err != nil {
			return fmt.Errorf("bad resolve %s on type %s: %s", name, typ, err)
		}
		object.Fields[name] = built
	}

	for _, i := range interfaces {
		interType := reflect.TypeOf(i.Type)
		if !typ.Implements(interType) {
			return fmt.Errorf("%s register interface %s but not implements", typ.Name(), interType.Name())
		}

		fields := make(map[string]*internal.Field, interType.NumMethod())
		for ii := 0; ii < interType.NumMethod(); ii++ {
			if f, ok := object.Fields[interType.Method(ii).Name]; !ok {
				return fmt.Errorf("%s implement interface %s, but not register into graphql", typ.Name(), interType.Name())
			} else {
				fields[interType.Method(ii).Name] = f
			}
		}

		inter := &internal.Interface{
			Name:   i.Name,
			Desc:   i.Desc,
			Fields: fields,
		}
		object.Interfaces[i.Name] = inter
	}

	return nil
}
