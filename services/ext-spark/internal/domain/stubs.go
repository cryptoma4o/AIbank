package domain

import (
	"crypto/sha256"
	"encoding/binary"
)

// SyntheticIntel генерирует детерминированную «спарковскую» аналитику по ИНН.
//
// Распределение financial_health (детерминированное от хэша):
//   - 60% green
//   - 30% yellow
//   - 10% red
//
// Sanctions выставляется в true примерно для 3% ИНН.
//
// Все строковые элементы — фиксированные шаблоны, никаких реальных персон/компаний.
func SyntheticIntel(inn string) SparkIntel {
	seed := hashSeed("spark:intel:" + inn)

	// FinancialHealth по hash%10: 0..5 green, 6..8 yellow, 9 red
	var health string
	switch seed % 10 {
	case 0, 1, 2, 3, 4, 5:
		health = "green"
	case 6, 7, 8:
		health = "yellow"
	default:
		health = "red"
	}

	// News summary — 2..4 фразы из фиксированного пула, выбираем по seed
	pool := []string{
		"Компания заключила контракт с государственным заказчиком",
		"Опубликована годовая бухгалтерская отчётность",
		"Изменён состав совета директоров",
		"Получена лицензия на новый вид деятельности",
		"Зарегистрирован новый филиал",
		"Завершён аудит финансовой отчётности",
		"Расширен штат сотрудников",
		"Подана заявка на государственную субсидию",
		"Открыт новый расчётный счёт в кредитной организации",
		"Зафиксировано изменение ОКВЭД",
	}
	count := int((seed/3)%3) + 2 // 2..4
	news := make([]string, 0, count)
	used := make(map[int]bool)
	for i := 0; i < count; i++ {
		idx := int((seed/uint64(7+i)) % uint64(len(pool)))
		// избежать дублей при коллизиях
		for used[idx] {
			idx = (idx + 1) % len(pool)
		}
		used[idx] = true
		news = append(news, pool[idx])
	}

	// Sanctions ~3%
	sanctions := (seed % 33) == 0

	// Налоги: 100k..50M рублей в год → 10M..5_000_000_000 копеек
	taxesRub := int64((seed/11)%50_000_000) + 100_000
	taxes := taxesRub * 100

	// Сотрудники: 1..500
	employees := int((seed/13)%500) + 1

	// Выручка: налоги * 50..200 (грубая прикидка)
	revMul := int64((seed/17)%150) + 50
	revenue := taxes * revMul

	return SparkIntel{
		INN:                 inn,
		FinancialHealth:     health,
		NewsSummary:         news,
		Sanctions:           sanctions,
		TaxesPaidKopecksYear: taxes,
		EmployeesCount:      employees,
		RevenueKopecksYear:  revenue,
		YearOfData:          2024,
	}
}

func hashSeed(s string) uint64 {
	h := sha256.Sum256([]byte(s))
	return binary.BigEndian.Uint64(h[:8])
}
