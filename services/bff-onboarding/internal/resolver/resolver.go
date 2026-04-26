package resolver

import (
	"aibank/bff-onboarding/internal/client"
	"github.com/graphql-go/graphql"
)

type Resolver struct {
	svc *client.ServiceClients
}

func New(svc *client.ServiceClients) *Resolver {
	return &Resolver{svc: svc}
}

func (r *Resolver) Application(p graphql.ResolveParams) (any, error) {
	id, _ := p.Args["id"].(string)
	return r.svc.GetApplication(p.Context, id)
}

func (r *Resolver) Client(p graphql.ResolveParams) (any, error) {
	id, _ := p.Args["id"].(string)
	tenantID, _ := p.Args["tenant_id"].(string)
	clientData, err := r.svc.GetClient(p.Context, tenantID, id)
	if err != nil {
		return nil, err
	}
	events, _ := r.svc.GetClientEvents(p.Context, tenantID, id)
	clientData["events"] = events
	return clientData, nil
}
