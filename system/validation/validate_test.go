package validation_test

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/schemabuilder"
	"github.com/unrotten/graphql/system"
	"github.com/unrotten/graphql/system/validation"
	"reflect"
	"strings"
	"testing"
)

var testSchema *system.Schema

type Being interface {
	Name(surname bool) string
}

type Mammal interface {
	Mother() Mammal
	Father() Mammal
}

type Pet interface {
	Name(surname bool) string
}

type Canine interface {
	Name(surname bool) string
	Mother() Mammal
	Father() Mammal
}

type DogCommand int

const (
	SIT  DogCommand = 0
	HEEL DogCommand = 1
	DOWN DogCommand = 2
)

type Dog struct {
	name            string
	Nickname        string `graphql:"nickname"`
	BarkVolume      int    `graphql:"barkVolume"`
	Barks           bool   `graphql:"barks"`
	DoesKnowCommand bool   `graphql:"doesKnowCommand"`
	IsHouseTrained  bool   `graphql:"isHouseTrained"`
	IsAtLocation    bool   `graphql:"isAtLocation"`
	mother          *Dog
	father          *Dog
}

func (d *Dog) Name(surname bool) string {
	if surname {
		return d.name
	}
	return ""
}

func (d *Dog) Mother() Mammal {
	return d.mother
}

func (d *Dog) Father() Mammal {
	return d.father
}

type Cat struct {
	name       string
	NickName   string   `graphql:"nickName"`
	Meows      bool     `graphql:"meows"`
	MeowVolume int      `graphql:"meowVolume"`
	FurColor   FurColor `graphql:"furColor"`
}

func (c *Cat) Name(surname bool) string {
	panic("implement me")
}

type CatDog struct {
	*Cat
	*Dog
}

type Intelligent interface {
	Iq() int
}

type Human struct {
	name      string
	Pets      []Pet   `graphql:"pets"`
	Relatives []Human `graphql:"relatives"`
	iq        int
}

func (h *Human) Name(surname bool) string {
	return h.name
}

func (h *Human) Iq() int {
	return h.iq
}

type Alien struct {
	iq      int
	name    string
	NumEyes int `graphql:"numEyes"`
}

func (a *Alien) Iq() int {
	return a.iq
}

func (a *Alien) Name(surname bool) string {
	return a.name
}

type DogOrHuman struct {
	*Dog
	*Human
}

type HumanOrAlien struct {
	*Human
	*Alien
}

type FurColor int

const (
	BROWN   FurColor = 0
	BLACK   FurColor = 1
	TAN     FurColor = 2
	SPOTTED FurColor = 3
	NO_FUR  FurColor = 4
	UNKNOWN FurColor = 5
)

type ComplexInput struct {
	RequiredField   bool     `graphql:"requiredField,nonnull"`
	NonNullField    bool     `graphql:"nonNullField,nonnull"`
	IntField        int      `graphql:"intField"`
	StringField     string   `graphql:"stringField"`
	BooleanField    bool     `graphql:"booleanField"`
	StringListField []string `graphql:"stringListField"`
}

type ComplicatedArgs struct{}

type InvalidScalar struct {
	Err error
}

type AnyScalar string

func (a *AnyScalar) UnmarshalJSON(b []byte) error {
	return nil
}

func init() {
	schema := schemabuilder.NewSchema()
	schema.Query()
	schema.Mutation()
	schema.Subscription()

	being := schema.Interface("Being", new(Being), nil, "")
	being.FieldFunc("name", "Name", "")

	mammal := schema.Interface("Mammal", new(Mammal), nil, "")
	mammal.FieldFunc("mother", "Mother", "")
	mammal.FieldFunc("father", "Father", "")

	pet := schema.Interface("Pet", new(Pet), nil, "")
	pet.FieldFunc("name", "Name", "")

	canine := schema.Interface("Canine", new(Canine), nil, "")
	canine.FieldFunc("name", "Name", "")
	canine.FieldFunc("mother", "Mother", "")
	canine.FieldFunc("father", "Father", "")

	schema.Enum("DogCommand", DogCommand(0), map[string]interface{}{
		"SIT":  SIT,
		"HEEL": HEEL,
		"DOWN": DOWN,
	}, "")

	dog := schema.Object("Dog", Dog{}, "")
	dog.InterfaceList(being, pet, mammal, canine)
	dog.FieldFunc("name", func(source *Dog, args struct {
		Surname bool `graphql:"surname"`
	}) string {
		return source.Name(args.Surname)
	}, "")
	dog.FieldFunc("doesKnowCommand", func(args struct {
		DogCommand DogCommand `graphql:"dogCommand"`
	}) bool {
		switch args.DogCommand {
		case SIT, HEEL, DOWN:
			return true
		default:
			return false
		}
	}, "")
	dog.FieldFunc("isHouseTrained", func(args struct {
		AtOtherHomes bool `graphql:"atOtherHomes"`
	}) bool {
		return !args.AtOtherHomes
	}, "")
	dog.FieldFunc("isAtLocation", func(args struct {
		X int `graphql:"x"`
		Y int `graphql:"y"`
	}) bool {
		return args.X == args.Y
	}, "")
	dog.FieldFunc("mother", func(source *Dog) *Dog { return source.Mother().(*Dog) }, "")
	dog.FieldFunc("father", func(source *Dog) *Dog { return source.Father().(*Dog) }, "")

	cat := schema.Object("Cat", Cat{}, "")
	cat.FieldFunc("name", func(source *Cat, args struct {
		Surname bool `graphql:"surname"`
	}) string {
		return source.Name(args.Surname)
	}, "")

	schema.Union("CatOrDog", CatDog{}, "")

	intelligent := schema.Interface("Intelligent", new(Intelligent), nil, "")
	intelligent.FieldFunc("iq", "Iq", "")

	human := schema.Object("Human", Human{}, "")
	human.InterfaceList(being, intelligent)
	human.FieldFunc("name", func(source *Human, args struct {
		Surname bool `graphql:"surname"`
	}) string {
		return source.Name(args.Surname)
	}, "")
	human.FieldFunc("iq", func(source *Human) int { return source.Iq() }, "")

	alien := schema.Object("Alien", Alien{}, "")
	alien.InterfaceList(being, intelligent)
	alien.FieldFunc("iq", func(source *Alien) int { return source.Iq() }, "")
	alien.FieldFunc("name", func(source *Alien, args struct {
		Surname bool `graphql:"surname"`
	}) string {
		return source.Name(args.Surname)
	}, "")

	schema.Union("DogOrHuman", DogOrHuman{}, "")

	schema.Union("HumanOrAlien", HumanOrAlien{}, "")

	schema.Enum("FurColor", FurColor(0), map[string]interface{}{
		"BROWN":   BROWN,
		"BLACK":   BLACK,
		"TAN":     TAN,
		"SPOTTED": SPOTTED,
		"NO_FUR":  NO_FUR,
		"UNKNOWN": UNKNOWN,
	}, "")

	complexInput := schema.InputObject("ComplexInput", ComplexInput{}, "")
	complexInput.FieldDefault("nonNullField", false)

	complicatedArgs := schema.Object("ComplicatedArgs", ComplicatedArgs{}, "")
	complicatedArgs.FieldFunc("intArgField", func(args struct {
		IntArg int `graphql:"intArg"`
	}) string {
		return fmt.Sprintf("%d", args.IntArg)
	}, "")
	complicatedArgs.FieldFunc("nonNullIntArgField", func(args struct {
		NonNullIntArg int `graphql:"nonNullIntArg,nonnull"`
	}) string {
		return fmt.Sprintf("%d", args.NonNullIntArg)
	}, "")
	complicatedArgs.FieldFunc("stringArgField", func(args struct {
		StringArg string `graphql:"stringArg"`
	}) string {
		return args.StringArg
	}, "")
	complicatedArgs.FieldFunc("booleanArgField", func(args struct {
		BooleanArg bool `graphql:"booleanArg"`
	}) string {
		return fmt.Sprintf("%t", args.BooleanArg)
	}, "")
	complicatedArgs.FieldFunc("enumArgField", func(args struct {
		EnumArg *FurColor `graphql:"enumArg"`
	}) string {
		return fmt.Sprintf("%d", args.EnumArg)
	}, "")
	complicatedArgs.FieldFunc("floatArgField", func(args struct {
		FloatArg float32 `graphql:"floatArg"`
	}) string {
		return fmt.Sprintf("%f", args.FloatArg)
	}, "")
	complicatedArgs.FieldFunc("idArgField", func(args struct {
		IdArg schemabuilder.Id `graphql:"idArg"`
	}) string {
		return fmt.Sprintf("%s", args.IdArg.Value)
	}, "")
	complicatedArgs.FieldFunc("stringListArgField", func(args struct {
		StringListArg []string `graphql:"stringListArg"`
	}) string {
		return strings.Join(args.StringListArg, ",")
	}, "")
	complicatedArgs.FieldFunc("stringListNonNullArgField", func(args struct {
		StringListNonNullArg []string `graphql:"stringListNonNullArg,,,itemNonnull"`
	}) string {
		return strings.Join(args.StringListNonNullArg, ",")
	}, "")
	complicatedArgs.FieldFunc("complexArgField", func(args struct {
		ComplexArg *ComplexInput `graphql:"complexArg"`
	}) string {
		return fmt.Sprintf("%v", args.ComplexArg)
	}, "")
	complicatedArgs.FieldFunc("multipleReqs", func(args struct {
		Req1 int `graphql:"req1,nonnull"`
		Req2 int `graphql:"req2,nonnull"`
	}) string {
		return fmt.Sprintf("%d,%d", args.Req1, args.Req2)
	}, "")
	complicatedArgs.FieldFunc("nonNullFieldWithDefault", func(args struct {
		Type int `graphql:"type,nonnull"`
	}) string {
		return fmt.Sprintf("%d", args.Type)
	}, "")
	complicatedArgs.FieldFunc("multipleOpts", func(args struct {
		Opt1 int `graphql:"opt1"`
		Opt2 int `graphql:"opt2"`
	}) string {
		return fmt.Sprintf("%d,%d", args.Opt1, args.Opt2)
	}, "")

	complicatedArgs.FieldFunc("multipleOptAndReq", func(args struct {
		Req1 int `graphql:"req1,nonnull"`
		Req2 int `graphql:"req2,nonnull"`
		Opt1 int `graphql:"opt1"`
		Opt2 int `graphql:"opt2"`
	}) string {
		return fmt.Sprintf("%d,%d", args.Req1, args.Req2)
	}, "")

	schema.Scalar("Invalid", InvalidScalar{}, "", func(value interface{}, dest reflect.Value) error {
		return fmt.Errorf(`invalid scalar is always invalid:"%s"`, value)
	})

	schema.Scalar("Any", AnyScalar(""), "")

	query := schema.Query()
	query.FieldFunc("human", func(args struct {
		Id schemabuilder.Id `graphql:"id"`
	}) Human {
		return Human{iq: args.Id.Value.(int)}
	}, "")
	query.FieldFunc("alien", func() Alien { return Alien{} }, "")
	query.FieldFunc("dog", func() Dog { return Dog{} }, "")
	query.FieldFunc("cat", func() Cat { return Cat{} }, "")
	query.FieldFunc("pet", func() Pet { return *new(Pet) }, "")
	query.FieldFunc("catOrDog", func() CatDog { return CatDog{Cat: &Cat{}} }, "")
	query.FieldFunc("dogOrHuman", func() DogOrHuman { return DogOrHuman{Dog: &Dog{}} }, "")
	query.FieldFunc("humanOrAlien", func() HumanOrAlien { return HumanOrAlien{Human: &Human{}} }, "")
	query.FieldFunc("complicatedArgs", func() ComplicatedArgs { return ComplicatedArgs{} }, "")
	query.FieldFunc("invalidArg", func(args struct {
		Arg InvalidScalar `graphql:"arg"`
	}) string {
		return fmt.Sprintf("%v", args.Arg.Err)
	}, "")
	query.FieldFunc("anyArg", func(args struct {
		Arg AnyScalar `graphql:"arg"`
	}) string {
		return string(args.Arg)
	}, "")

	schema.Directive("onQuery", []string{"QUERY"}, nil, "")
	schema.Directive("onMutation	", []string{"MUTATION"}, nil, "")
	schema.Directive("onSubscription	", []string{"SUBSCRIPTION"}, nil, "")
	schema.Directive("onField", []string{"FIELD"}, nil, "")
	schema.Directive("onFragmentDefinition", []string{"FRAGMENT_DEFINITION"}, nil, "")
	schema.Directive("onFragmentSpread", []string{"FRAGMENT_SPREAD"}, nil, "")
	schema.Directive("onInlineFragment", []string{"INLINE_FRAGMENT"}, nil, "")
	schema.Directive("onVariableDefinition", []string{"VARIABLE_DEFINITION"}, nil, "")

	testSchema = schema.MustBuild()
}

func TestValidate(t *testing.T) {
	var Nil *errors.GraphQLError
	t.Run("validates queries", func(t *testing.T) {
		doc, err := system.Parse(`
      query {
        catOrDog {
          ... on Cat {
            furColor
          }
          ... on Dog {
            isHouseTrained
          }
        }
      }
    `)
		assert.Equal(t, Nil, err)
		assert.Zero(t, validation.Validate(testSchema, doc, nil, 50))
	})

	t.Run("detects bad scalar parse", func(t *testing.T) {
		doc, err := system.Parse(`
      query {
        invalidArg(arg: "bad value")
      }
    `)
		assert.Equal(t, Nil, err)
		assert.Equal(t, []*errors.GraphQLError{{
			Message:   `Expected value of type "Invalid", found "bad value"; invalid scalar is always invalid:"bad value"`,
			Locations: []errors.Location{{3, 25}},
			Rule:      "ValuesOfCorrectType",
		}}, validation.Validate(testSchema, doc, nil, 50))
	})

	t.Run("validation.Validate: Limit maximum number of validation errors", func(t *testing.T) {

		const query = `
    {
      firstUnknownField
      secondUnknownField
      thirdUnknownField
    }
  `
		doc, err := system.Parse(query)
		assert.Equal(t, Nil, err)

		validateDocument := func(max int) []*errors.GraphQLError {
			return validation.Validate(testSchema, doc, nil, max)
		}
		invalidFieldError := func(name string, rule string, loc errors.Location) *errors.GraphQLError {
			return &errors.GraphQLError{
				Message:   fmt.Sprintf(`Cannot query field "%s" on type "Query".`, name),
				Locations: []errors.Location{loc},
			}
		}
		t.Run("when maxErrors is equal to number of errors", func(t *testing.T) {
			errs := validateDocument(3)
			assert.Equal(t, []*errors.GraphQLError{
				invalidFieldError("firstUnknownField", "FieldsOnCorrectType", errors.Location{3, 7}),
				invalidFieldError("secondUnknownField", "FieldsOnCorrectType", errors.Location{4, 7}),
				invalidFieldError("thirdUnknownField", "FieldsOnCorrectType", errors.Location{5, 7}),
			}, errs)
		})
	})
}
