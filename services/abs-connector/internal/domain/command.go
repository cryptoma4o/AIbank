package domain

type CommandType string

const (
	CmdOpenAccount    CommandType = "OpenAccount"
	CmdCloseAccount   CommandType = "CloseAccount"
	CmdGetAccountInfo CommandType = "GetAccountInfo"
	CmdCreateClient   CommandType = "CreateClient"
)

type ABSCommand struct {
	IdempotencyKey string         `json:"idempotency_key"`
	TenantID       string         `json:"tenant_id"`
	Command        CommandType    `json:"command"`
	Payload        map[string]any `json:"payload"`
}

type ABSResponse struct {
	IdempotencyKey string         `json:"idempotency_key"`
	Success        bool           `json:"success"`
	Data           map[string]any `json:"data,omitempty"`
	Error          string         `json:"error,omitempty"`
	AdapterUsed    string         `json:"adapter_used"`
}
