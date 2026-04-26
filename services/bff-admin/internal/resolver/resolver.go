package resolver

import (
	"aibank/bff-admin/internal/client"
	"github.com/graphql-go/graphql"
)

type Resolver struct {
	svc *client.AdminClients
}

func New(svc *client.AdminClients) *Resolver {
	return &Resolver{svc: svc}
}

func (r *Resolver) Tenant(p graphql.ResolveParams) (any, error) {
	id, _ := p.Args["id"].(string)
	return r.svc.GetTenant(p.Context, id)
}

func (r *Resolver) AuditEvents(p graphql.ResolveParams) (any, error) {
	tenantID, _ := p.Args["tenant_id"].(string)
	limit := 50
	if l, ok := p.Args["limit"].(int); ok && l > 0 {
		limit = l
	}
	return r.svc.ListAuditEvents(p.Context, tenantID, limit)
}

func (r *Resolver) Client(p graphql.ResolveParams) (any, error) {
	tenantID, _ := p.Args["tenant_id"].(string)
	id, _ := p.Args["id"].(string)
	return r.svc.GetClient(p.Context, tenantID, id)
}

func (r *Resolver) UBOGraph(p graphql.ResolveParams) (any, error) {
	tenantID, _ := p.Args["tenant_id"].(string)
	appID, _ := p.Args["app_id"].(string)
	return r.svc.GetUBOGraph(p.Context, tenantID, appID)
}
