package scorer

// MLScorer is a stub for the CatBoost model integration.
// Real implementation loads a .cbm model file via cgo bindings.
type MLScorer struct{}

func NewMLScorer() *MLScorer { return &MLScorer{} }

// Score returns nil when ML model is unavailable (Pre-MVP stub).
func (m *MLScorer) Score(_ map[string]any) *int { return nil }
