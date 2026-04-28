package domain

import "testing"

func TestLoadPricelist_Basic(t *testing.T) {
	t.Parallel()

	p := LoadPricelist(TierBasic)
	if p.Tier != TierBasic {
		t.Fatalf("expected tier basic, got %s", p.Tier)
	}
	mustEqualPrice(t, p, EventTypeAccountOpenedIP, 20000)   // 200 ₽
	mustEqualPrice(t, p, EventTypeAccountOpenedLLC, 80000)  // 800 ₽
	mustEqualPrice(t, p, EventTypeAccountOpenedJSC, 200000) // 2 000 ₽
	mustEqualPrice(t, p, EventTypeUBOCheckExecuted, 15000)
	mustEqualPrice(t, p, EventTypeManualReviewDone, 30000)
}

func TestLoadPricelist_Premium(t *testing.T) {
	t.Parallel()

	p := LoadPricelist(TierPremium)
	if p.Tier != TierPremium {
		t.Fatalf("expected tier premium, got %s", p.Tier)
	}
	mustEqualPrice(t, p, EventTypeAccountOpenedIP, 30000)
	mustEqualPrice(t, p, EventTypeAccountOpenedLLC, 110000)
	mustEqualPrice(t, p, EventTypeAccountOpenedJSC, 280000)
}

func TestLoadPricelist_Enterprise(t *testing.T) {
	t.Parallel()

	p := LoadPricelist(TierEnterprise)
	if p.Tier != TierEnterprise {
		t.Fatalf("expected tier enterprise, got %s", p.Tier)
	}
	mustEqualPrice(t, p, EventTypeAccountOpenedIP, 40000)   // 400 ₽
	mustEqualPrice(t, p, EventTypeAccountOpenedLLC, 150000) // 1 500 ₽
	mustEqualPrice(t, p, EventTypeAccountOpenedJSC, 400000) // 4 000 ₽
}

func TestLoadPricelist_UnknownTierFallsBackToBasic(t *testing.T) {
	t.Parallel()

	p := LoadPricelist("unknown-tier")
	if p.Tier != TierBasic {
		t.Fatalf("expected fallback to basic, got %s", p.Tier)
	}
}

func TestPrice_UnknownEventTypeIsZero(t *testing.T) {
	t.Parallel()

	p := LoadPricelist(TierBasic)
	got := p.Price("unknown.event.type", nil)
	if got != 0 {
		t.Errorf("expected 0 for unknown event_type, got %d", got)
	}
}

func mustEqualPrice(t *testing.T, p *Pricelist, eventType string, want int64) {
	t.Helper()
	got := p.Price(eventType, nil)
	if got != want {
		t.Errorf("Price(%s) on %s = %d, want %d", eventType, p.Tier, got, want)
	}
}
