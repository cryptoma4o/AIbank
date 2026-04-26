package domain

import "time"

type NodeType string

const (
	NodeTypePerson  NodeType = "person"
	NodeTypeCompany NodeType = "company"
)

type UBONode struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	AppID     string    `json:"app_id"`
	NodeType  NodeType  `json:"node_type"`
	Name      string    `json:"name"`
	INN       string    `json:"inn,omitempty"`
	Passport  string    `json:"passport,omitempty"` // masked in response
	IsUBO     bool      `json:"is_ubo"`             // effective stake ≥25%
	Stake     float64   `json:"stake"`              // direct + indirect stake %
	CreatedAt time.Time `json:"created_at"`
}

type UBOEdge struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	AppID       string    `json:"app_id"`
	FromNodeID  string    `json:"from_node_id"`
	ToNodeID    string    `json:"to_node_id"`
	DirectStake float64   `json:"direct_stake"` // ownership % (0-100)
	CreatedAt   time.Time `json:"created_at"`
}

type UBOGraph struct {
	AppID     string    `json:"app_id"`
	TenantID  string    `json:"tenant_id"`
	Nodes     []UBONode `json:"nodes"`
	Edges     []UBOEdge `json:"edges"`
	UBOs      []UBONode `json:"ubos"` // nodes where is_ubo=true
	CreatedAt time.Time `json:"created_at"`
}
