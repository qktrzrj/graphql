package resource

import "io/ioutil"

var (
	BigSchemaSDL                 []byte
	BigSchemaIntrospectionResult []byte
	KitchenSinkSDL               []byte
	KitchenSinkQuery             []byte
)

func init() {
	BigSchemaSDL, _ = ioutil.ReadFile("../resource/github-schema.graphql")
	BigSchemaIntrospectionResult, _ = ioutil.ReadFile("../resource/github-schema.json")
	KitchenSinkSDL, _ = ioutil.ReadFile("../resource/schema-kitchen-sink.graphql")
	KitchenSinkQuery, _ = ioutil.ReadFile("../resource/kitchen-sink.graphql")
}
