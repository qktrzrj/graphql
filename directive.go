package graphql

import (
	"context"
	"errors"
	"reflect"
)

type DirectiveLocation string

const (
	// Operations
	DirectiveLocationQuery              DirectiveLocation = "QUERY"
	DirectiveLocationMutation           DirectiveLocation = "MUTATION"
	DirectiveLocationSubscription       DirectiveLocation = "SUBSCRIPTION"
	DirectiveLocationField              DirectiveLocation = "FIELD"
	DirectiveLocationFragmentDefinition DirectiveLocation = "FRAGMENT_DEFINITION"
	DirectiveLocationFragmentSpread     DirectiveLocation = "FRAGMENT_SPREAD"
	DirectiveLocationInlineFragment     DirectiveLocation = "INLINE_FRAGMENT"

	// Schema Definitions
	DirectiveLocationSchema               DirectiveLocation = "SCHEMA"
	DirectiveLocationScalar               DirectiveLocation = "SCALAR"
	DirectiveLocationObject               DirectiveLocation = "OBJECT"
	DirectiveLocationFieldDefinition      DirectiveLocation = "FIELD_DEFINITION"
	DirectiveLocationArgumentDefinition   DirectiveLocation = "ARGUMENT_DEFINITION"
	DirectiveLocationInterface            DirectiveLocation = "INTERFACE"
	DirectiveLocationUnion                DirectiveLocation = "UNION"
	DirectiveLocationEnum                 DirectiveLocation = "ENUM"
	DirectiveLocationEnumValue            DirectiveLocation = "ENUM_VALUE"
	DirectiveLocationInputObject          DirectiveLocation = "INPUT_OBJECT"
	DirectiveLocationInputFieldDefinition DirectiveLocation = "INPUT_FIELD_DEFINITION"
)

var (
	Skip = errors.New("skip")
)

// DefaultDeprecationReason Constant string used for default reason for a deprecation.
const DefaultDeprecationReason = "No longer supported"

type ResolveChain func(FieldResolve) FieldResolve
type DirectiveFn func(input interface{}) ResolveChain

// Directive structs are used by the GraphQL runtime as a way of modifying execution
// behavior. Type system creators will usually not create these directly.
type Directive struct {
	Name        string
	Description string
	Locations   []DirectiveLocation
}

type DirectiveBuilder struct {
	Name        string
	Description string
	Locations   []DirectiveLocation
	Args        *FieldInputBuilder
	DirectiveFn DirectiveFn
}

// IncludeDirective is used to conditionally include fields or fragments.
var IncludeDirective = &DirectiveBuilder{
	Name:        "include",
	Description: "Directs the executor to include this field or fragment only when the `if` argument is true.",
	Locations: []DirectiveLocation{
		DirectiveLocationField,
		DirectiveLocationFragmentSpread,
		DirectiveLocationInlineFragment,
	},
	Args: &FieldInputBuilder{
		Name:        "if",
		Description: "Included when true.",
		Type:        reflect.TypeOf(bool(false)),
	},
	DirectiveFn: func(input interface{}) ResolveChain {
		return func(resolve FieldResolve) FieldResolve {
			return func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
				if !input.(bool) {
					return nil, Skip
				}
				return resolve(ctx, source, args)
			}
		}
	},
}

// SkipDirective Used to conditionally skip (exclude) fields or fragments.
var SkipDirective = &DirectiveBuilder{
	Name:        "skip",
	Description: "Directs the executor to skip this field or fragment when the `if` argument is true.",
	Locations: []DirectiveLocation{
		DirectiveLocationField,
		DirectiveLocationFragmentSpread,
		DirectiveLocationInlineFragment,
	},
	Args: &FieldInputBuilder{
		Name:        "if",
		Description: "Skipped when true.",
		Type:        reflect.TypeOf(bool(false)),
	},
	DirectiveFn: func(input interface{}) ResolveChain {
		return func(resolve FieldResolve) FieldResolve {
			return func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
				if input.(bool) {
					return nil, Skip
				}
				return resolve(ctx, source, args)
			}
		}
	},
}

// DeprecatedDirective  Used to declare element of a GraphQL schema as deprecated.
var DeprecatedDirective = &DirectiveBuilder{
	Name:        "deprecated",
	Description: "Marks an element of a GraphQL schema as no longer supported.",
	Locations: []DirectiveLocation{
		DirectiveLocationFieldDefinition,
		DirectiveLocationEnumValue,
	},
	Args: &FieldInputBuilder{
		Name: "reason",
		Description: "Explains why this element was deprecated, usually also including a " +
			"suggestion for how to access supported similar data. Formatted" +
			"in [Markdown](https://daringfireball.net/projects/markdown/).",
		Type:         reflect.TypeOf(string("")),
		DefaultValue: DefaultDeprecationReason,
	},
	DirectiveFn: func(input interface{}) ResolveChain {
		return func(resolve FieldResolve) FieldResolve {
			return func(ctx context.Context, source, args interface{}) (res interface{}, err error) {
				return resolve(ctx, source, args)
			}
		}
	},
}
