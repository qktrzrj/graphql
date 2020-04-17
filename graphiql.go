package graphql

import (
	"html/template"
	"net/http"
)

var URL = "query"

func GraphiQLHandler(url ...string) http.HandlerFunc {
	if len(url) > 0 {
		URL = url[0]
	}
	return graphiQLHandler
}

func graphiQLHandler(w http.ResponseWriter, r *http.Request) {
	t := template.New("GraphiQL")
	t, err := t.Parse(page)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = t.ExecuteTemplate(w, "index", struct {
		URL string
	}{URL: URL})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

const page = `
{{ define "index" }}
<!--
The request to this GraphQL server provided the header "Accept: text/html"
and as a result has been presented GraphiQL - an in-browser IDE for
exploring GraphQL.

If you wish to receive JSON, provide the header "Accept: application/json" or
add "&raw" to the end of the URL within a browser.
-->
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8" />
    <title>GraphiQL</title>
    <meta name="robots" content="noindex" />
    <meta name="referrer" content="origin">
    <link href="https://cdnjs.cloudflare.com/ajax/libs/graphiql/0.11.11/graphiql.min.css" rel="stylesheet"/>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/es6-promise/4.1.1/es6-promise.auto.min.js"></script>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/fetch/2.0.3/fetch.min.js"></script>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/react/16.2.0/umd/react.production.min.js"></script>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/react-dom/16.2.0/umd/react-dom.production.min.js"></script>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/graphiql/0.11.11/graphiql.min.js"></script>
</head>
<body style="width: 100%; height: 100%; margin: 0; overflow: hidden;">
<div id="graphiql" style="height: 100vh;">Loading...</div>
<script>
    function graphQLFetcher(graphQLParams) {
        const uri = "{{.URL}}";
        return fetch(uri, {
            method:"post",
            headers: {
                'Accept': 'application/json',
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(graphQLParams),
            credentials: 'include',
        }).then(function (response) {
            return response.text();
        }).then(function (responseBody) {
            try {
                return JSON.parse(responseBody);
            } catch (error) {
                return responseBody;
            }
        });
    }

    // Render <GraphiQL /> into the body.
    ReactDOM.render(
        React.createElement(GraphiQL, {
            fetcher: graphQLFetcher,
        }),
        document.getElementById('graphiql')
    );
</script>
</body>
</html>
{{ end }}
`
