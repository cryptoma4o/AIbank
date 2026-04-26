package schema

import (
	"aibank/bff-onboarding/internal/resolver"

	"github.com/graphql-go/graphql"
)

func Build(r *resolver.Resolver) (graphql.Schema, error) {
	applicationType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Application",
		Fields: graphql.Fields{
			"id":        &graphql.Field{Type: graphql.String},
			"tenant_id": &graphql.Field{Type: graphql.String},
			"status":    &graphql.Field{Type: graphql.String},
			"inn":       &graphql.Field{Type: graphql.String},
			"ogrn":      &graphql.Field{Type: graphql.String},
		},
	})

	clientEventType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ClientEvent",
		Fields: graphql.Fields{
			"id":          &graphql.Field{Type: graphql.String},
			"category":    &graphql.Field{Type: graphql.String},
			"event_type":  &graphql.Field{Type: graphql.String},
			"resource_id": &graphql.Field{Type: graphql.String},
			"occurred_at": &graphql.Field{Type: graphql.String},
		},
	})

	clientType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Client",
		Fields: graphql.Fields{
			"id":        &graphql.Field{Type: graphql.String},
			"inn":       &graphql.Field{Type: graphql.String},
			"full_name": &graphql.Field{Type: graphql.String},
			"status":    &graphql.Field{Type: graphql.String},
			"events":    &graphql.Field{Type: graphql.NewList(clientEventType)},
		},
	})

	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"application": {
				Type: applicationType,
				Args: graphql.FieldConfigArgument{
					"id": {Type: graphql.NewNonNull(graphql.String)},
				},
				Resolve: r.Application,
			},
			"client": {
				Type: clientType,
				Args: graphql.FieldConfigArgument{
					"id":        {Type: graphql.NewNonNull(graphql.String)},
					"tenant_id": {Type: graphql.NewNonNull(graphql.String)},
				},
				Resolve: r.Client,
			},
		},
	})

	return graphql.NewSchema(graphql.SchemaConfig{Query: queryType})
}
