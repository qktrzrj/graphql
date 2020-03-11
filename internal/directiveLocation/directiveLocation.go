package directiveLocation

const (
	// Operations
	Query              = "QUERY"
	Mutation           = "MUTATION"
	Subscription       = "SUBSCRIPTION"
	Field              = "FIELD"
	FragmentDefinition = "FRAGMENT_DEFINITION"
	FragmentSpread     = "FRAGMENT_SPREAD"
	InlineFragment     = "INLINE_FRAGMENT"

	// Schema Definitions
	Schema               = "SCHEMA"
	Scalar               = "SCALAR"
	Object               = "OBJECT"
	FieldDefinition      = "FIELD_DEFINITION"
	ArgumentDefinition   = "ARGUMENT_DEFINITION"
	Interface            = "INTERFACE"
	Union                = "UNION"
	Enum                 = "ENUM"
	EnumValue            = "ENUM_VALUE"
	InputObject          = "INPUT_OBJECT"
	InputFieldDefinition = "INPUT_FIELD_DEFINITION"
)
