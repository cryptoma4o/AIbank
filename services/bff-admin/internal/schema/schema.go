package schema

import (
	"aibank/bff-admin/internal/resolver"

	"github.com/graphql-go/graphql"
)

func Build(r *resolver.Resolver) (graphql.Schema, error) {
	tenantType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Tenant",
		Fields: graphql.Fields{
			"id":   &graphql.Field{Type: graphql.String},
			"name": &graphql.Field{Type: graphql.String},
			"slug": &graphql.Field{Type: graphql.String},
		},
	})

	auditEventType := graphql.NewObject(graphql.ObjectConfig{
		Name: "AuditEvent",
		Fields: graphql.Fields{
			"id":          &graphql.Field{Type: graphql.String},
			"event_type":  &graphql.Field{Type: graphql.String},
			"actor_id":    &graphql.Field{Type: graphql.String},
			"resource_id": &graphql.Field{Type: graphql.String},
			"occurred_at": &graphql.Field{Type: graphql.String},
		},
	})

	uboNodeType := graphql.NewObject(graphql.ObjectConfig{
		Name: "UBONode",
		Fields: graphql.Fields{
			"id":        &graphql.Field{Type: graphql.String},
			"name":      &graphql.Field{Type: graphql.String},
			"node_type": &graphql.Field{Type: graphql.String},
			"stake":     &graphql.Field{Type: graphql.Float},
			"is_ubo":    &graphql.Field{Type: graphql.Boolean},
		},
	})

	uboGraphType := graphql.NewObject(graphql.ObjectConfig{
		Name: "UBOGraph",
		Fields: graphql.Fields{
			"app_id": &graphql.Field{Type: graphql.String},
			"nodes":  &graphql.Field{Type: graphql.NewList(uboNodeType)},
			"ubos":   &graphql.Field{Type: graphql.NewList(uboNodeType)},
		},
	})

	clientType := graphql.NewObject(graphql.ObjectConfig{
		Name: "AdminClient",
		Fields: graphql.Fields{
			"id":        &graphql.Field{Type: graphql.String},
			"inn":       &graphql.Field{Type: graphql.String},
			"full_name": &graphql.Field{Type: graphql.String},
			"status":    &graphql.Field{Type: graphql.String},
			"type":      &graphql.Field{Type: graphql.String},
		},
	})

	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"tenant": {
				Type: tenantType,
				Args: graphql.FieldConfigArgument{
					"id": {Type: graphql.NewNonNull(graphql.String)},
				},
				Resolve: r.Tenant,
			},
			"auditEvents": {
				Type: graphql.NewList(auditEventType),
				Args: graphql.FieldConfigArgument{
					"tenant_id": {Type: graphql.NewNonNull(graphql.String)},
					"limit":     {Type: graphql.Int},
				},
				Resolve: r.AuditEvents,
			},
			"client": {
				Type: clientType,
				Args: graphql.FieldConfigArgument{
					"tenant_id": {Type: graphql.NewNonNull(graphql.String)},
					"id":        {Type: graphql.NewNonNull(graphql.String)},
				},
				Resolve: r.Client,
			},
			"uboGraph": {
				Type: uboGraphType,
				Args: graphql.FieldConfigArgument{
					"tenant_id": {Type: graphql.NewNonNull(graphql.String)},
					"app_id":    {Type: graphql.NewNonNull(graphql.String)},
				},
				Resolve: r.UBOGraph,
			},
		},
	})

	return graphql.NewSchema(graphql.SchemaConfig{Query: queryType})
}
