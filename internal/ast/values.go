package ast

import (
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/internal/kinds"
)

// Field and directive arguments accept input values of various literal primitives;
// input values can be scalars, enumeration values, lists, or input objects.
//
// If not defined as constant (for example, in DefaultValue), input values can be specified as a variable.
// List and inputs objects may also contain variables (unless defined to be constant).
type Value interface {
	Node
	GetValue() interface{}
}

var _ Value = (*Variable)(nil)
var _ Value = (*IntValue)(nil)
var _ Value = (*FloatValue)(nil)
var _ Value = (*StringValue)(nil)

var _ Value = (*NullValue)(nil)
var _ Value = (*BooleanValue)(nil)
var _ Value = (*EnumValue)(nil)
var _ Value = (*ListValue)(nil)
var _ Value = (*ObjectValue)(nil)

// An IntValue is specified without a decimal point or exponent but may be negative (ex. -123). It must not have any leading 0.
//
// An IntValue must not be followed by a Digit.
// In other words, an IntValue token is always the longest possible valid sequence.
// The source characters 12 cannot be interpreted as two tokens since 1 is followed by the Digit 2.
// This also means the source 00 is invalid since it can neither be interpreted as a single token nor two 0 tokens.
//
// An IntValue must not be followed by a . or NameStart. If either .
// or ExponentIndicator follows then the token must only be interpreted as a possible FloatValue.
// No other NameStart character can follow. For example the sequences 0x123 and 123L have no valid lexical representations.
type IntValue struct {
	Value string
	Loc   errors.Location
}

func (i *IntValue) Kind() string {
	return kinds.IntValue
}

func (i *IntValue) Location() errors.Location {
	return i.Loc
}

func (i *IntValue) GetValue() interface{} { return i.Value }

// A FloatValue includes either a decimal point (ex. 1.0) or
// an exponent (ex. 1e50) or both (ex. 6.0221413e23) and may be negative.
// Like IntValue, it also must not have any leading 0.
//
// A FloatValue must not be followed by a Digit.
// In other words, a FloatValue token is always the longest possible valid sequence.
// The source characters 1.23 cannot be interpreted as two tokens since 1.2 is followed by the Digit 3.
//
// A FloatValue must not be followed by a .. For example, the sequence 1.23.4 cannot be interpreted as two tokens (1.2, 3.4).
//
// A FloatValue must not be followed by a NameStart. For example the sequence 0x1.2p3 has no valid lexical representation.
type FloatValue struct {
	Value string
	Loc   errors.Location
}

func (f *FloatValue) Kind() string {
	return kinds.FloatValue
}

func (f *FloatValue) Location() errors.Location {
	return f.Loc
}

func (f *FloatValue) GetValue() interface{} { return f.Value }

// NullValue
type NullValue struct {
	Loc errors.Location
}

func (n *NullValue) Kind() string {
	return "null"
}

func (n *NullValue) Location() errors.Location {
	return n.Loc
}

func (n *NullValue) GetValue() interface{} {
	return "null"
}

// The two keywords true and false represent the two boolean values.
type BooleanValue struct {
	Value bool
	Loc   errors.Location
}

func (b *BooleanValue) Kind() string {
	return kinds.BooleanValue
}

func (b *BooleanValue) Location() errors.Location {
	return b.Loc
}

func (b *BooleanValue) GetValue() interface{} { return b.Value }

// Strings are sequences of characters wrapped in quotation marks (U+0022). (ex. "Hello World").
// White space and other otherwise‐ignored characters are significant within a string value.
//
// The empty string "" must not be followed by another " otherwise it would be interpreted as the beginning of a block string.
// As an example, the source """""" can only be interpreted as a single empty block string and not three empty strings.
type StringValue struct {
	Value string
	Loc   errors.Location
}

func (s *StringValue) Kind() string {
	return kinds.StringValue
}

func (s *StringValue) Location() errors.Location {
	return s.Loc
}

func (s *StringValue) GetValue() interface{} { return s.Value }

//// Null values are represented as the keyword null.
////
//// GraphQL has two semantically different ways to represent the lack of a value:
////
//// Explicitly providing the literal value: null.
//// Implicitly not providing a value at all.
//// For example, these two field calls are similar, but are not identical:
////
//// {
////   field(arg: null)
////   field
//// }
//// The first has explicitly provided null to the argument “arg”,
//// while the second has implicitly not provided a value to the argument “arg”.
//// These two forms may be interpreted differently.
//// For example, a mutation representing deleting a field vs not altering a field, respectively.
//// Neither form may be used for an input expecting a Non‐Null type.
//type NullValue struct {
//	Loc errors.Location
//}
//
//func (n *NullValue) Kind() string {
//	return "null"
//}
//
//func (n *NullValue) Location() errors.Location {
//	return n.Loc
//}
//
//func (n *NullValue) GetValue() interface{} {
//	return "null"
//}

// Enum values are represented as unquoted names (ex. MOBILE_WEB).
// It is recommended that Enum values be “all caps”.
// Enum values are only used in contexts where the precise enumeration type is known.
// Therefore it’s not necessary to supply an enumeration type name in the literal.
type EnumValue struct {
	Value string
	Loc   errors.Location
}

func (e *EnumValue) Kind() string {
	return kinds.EnumValue
}

func (e *EnumValue) Location() errors.Location {
	return e.Loc
}

func (e *EnumValue) GetValue() interface{} {
	return e.Value
}

// Lists are ordered sequences of values wrapped in square‐brackets [ ].
// The values of a List literal may be any value literal or variable (ex. [1, 2, 3]).
//
// Commas are optional throughout GraphQL so trailing commas are allowed and repeated commas do not represent missing values.
type ListValue struct {
	Loc    errors.Location
	Values []Value
}

func (l *ListValue) Kind() string {
	return kinds.ListValue
}

func (l *ListValue) Location() errors.Location {
	return l.Loc
}

func (l *ListValue) GetValue() interface{} {
	return l.Values
}

// Input object literal values are unordered lists of keyed input values wrapped in curly‐braces { }.
// The values of an object literal may be any input value literal or variable (ex. { name: "Hello world", score: 1.0 }).
// We refer to literal representation of input objects as “object literals.”
//
// Input object fields are unordered
// Input object fields may be provided in any syntactic order and maintain identical semantic meaning.
//
// These two queries are semantically identical:
//
// {
//   nearestThing(location: { lon: 12.43, lat: -53.211 })
// }
// {
//   nearestThing(location: { lat: -53.211, lon: 12.43 })
// }
type ObjectValue struct {
	Fields []*ObjectField
	Loc    errors.Location
}

func (o *ObjectValue) Kind() string {
	return kinds.ObjectValue
}

func (o *ObjectValue) Location() errors.Location {
	return o.Loc
}

func (o *ObjectValue) GetValue() interface{} {
	return o.Fields
}

type ObjectField struct {
	Name  *Named
	Value Value
	Loc   errors.Location
}

func (o *ObjectField) Kind() string {
	return kinds.ObjectField
}

func (o *ObjectField) Location() errors.Location {
	return o.Loc
}
