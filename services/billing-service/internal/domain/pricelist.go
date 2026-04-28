package domain

import (
	"log/slog"
)

// Tier — тарифная категория тенанта. Соответствует contract.pricing_tier
// в tenant config; неизвестный/пустой tier трактуется как TierBasic.
type Tier string

const (
	TierBasic      Tier = "basic"
	TierPremium    Tier = "premium"
	TierEnterprise Tier = "enterprise"
)

// Pricelist — таблица цен в копейках для одной тарифной категории.
// Цены — целочисленные kopecks (никогда float, см. ADR-0010 § 3).
//
// Диапазоны (product-vision.md § 5):
//
//	Открытый счёт ИП:  200–400 ₽   → 20 000–40 000 kopecks
//	Открытый счёт ООО: 800–1500 ₽  → 80 000–150 000 kopecks
//	Открытый счёт АО:  2000–4000 ₽ → 200 000–400 000 kopecks
//	Проверка УБО:      отдельная позиция
type Pricelist struct {
	Tier   Tier             `json:"tier"`
	Prices map[string]int64 `json:"prices"`
}

// Price возвращает unit_price в копейках для event_type. Для неизвестных
// типов возвращает 0 и пишет warn-лог, но не считает это ошибкой —
// неизвестное событие записывается с amount=0 для последующего ручного
// реклассифицирования (ADR-0010 § 6 «Ad-hoc корректировки»).
func (p *Pricelist) Price(eventType string, log *slog.Logger) int64 {
	if price, ok := p.Prices[eventType]; ok {
		return price
	}
	if log != nil {
		log.Warn("неизвестный event_type — цена 0 (требуется ручная корректировка)",
			"event_type", eventType, "tier", p.Tier)
	}
	return 0
}

// LoadPricelist возвращает дефолтный price-лист для указанного tier.
// Для unknown/empty tier — TierBasic (защитный fallback per ADR-0010 § 6).
func LoadPricelist(tier Tier) *Pricelist {
	switch tier {
	case TierPremium:
		return &Pricelist{Tier: TierPremium, Prices: premiumPrices()}
	case TierEnterprise:
		return &Pricelist{Tier: TierEnterprise, Prices: enterprisePrices()}
	default:
		return &Pricelist{Tier: TierBasic, Prices: basicPrices()}
	}
}

// basicPrices — нижняя граница диапазонов из product-vision.md § 5.
func basicPrices() map[string]int64 {
	return map[string]int64{
		EventTypeAccountOpenedIP:  20000,  // 200 ₽
		EventTypeAccountOpenedLLC: 80000,  // 800 ₽
		EventTypeAccountOpenedJSC: 200000, // 2 000 ₽
		EventTypeUBOCheckExecuted: 15000,  // 150 ₽
		EventTypeManualReviewDone: 30000,  // 300 ₽
		EventTypeDocumentParsed:   500,    // 5 ₽
		EventTypeRiskScored:       1000,   // 10 ₽
	}
}

// premiumPrices — середина диапазонов.
func premiumPrices() map[string]int64 {
	return map[string]int64{
		EventTypeAccountOpenedIP:  30000,  // 300 ₽
		EventTypeAccountOpenedLLC: 110000, // 1 100 ₽
		EventTypeAccountOpenedJSC: 280000, // 2 800 ₽
		EventTypeUBOCheckExecuted: 25000,  // 250 ₽
		EventTypeManualReviewDone: 50000,  // 500 ₽
		EventTypeDocumentParsed:   1000,   // 10 ₽
		EventTypeRiskScored:       2000,   // 20 ₽
	}
}

// enterprisePrices — верхняя граница диапазонов.
func enterprisePrices() map[string]int64 {
	return map[string]int64{
		EventTypeAccountOpenedIP:  40000,  // 400 ₽
		EventTypeAccountOpenedLLC: 150000, // 1 500 ₽
		EventTypeAccountOpenedJSC: 400000, // 4 000 ₽
		EventTypeUBOCheckExecuted: 50000,  // 500 ₽
		EventTypeManualReviewDone: 80000,  // 800 ₽
		EventTypeDocumentParsed:   1500,   // 15 ₽
		EventTypeRiskScored:       3000,   // 30 ₽
	}
}
