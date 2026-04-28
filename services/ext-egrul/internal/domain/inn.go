package domain

import "errors"

// ErrInvalidINN — формат ИНН не соответствует 10 (ЮЛ) или 12 (ИП/ФЛ) цифрам.
var ErrInvalidINN = errors.New("invalid INN format: expected 10 (legal entity) or 12 (individual) digits")

// ErrInvalidOGRN — формат ОГРН не соответствует 13 (ЮЛ) или 15 (ИП) цифрам.
var ErrInvalidOGRN = errors.New("invalid OGRN format: expected 13 (legal entity) or 15 (individual) digits")

// ValidateINN проверяет формат ИНН: 10 или 12 цифр, только цифры.
//
// Контрольная сумма не проверяется — это задача source-of-truth API ФНС.
// Здесь только базовый sanity check, чтобы не ходить в кэш/стаб с мусором.
func ValidateINN(inn string) error {
	if len(inn) != 10 && len(inn) != 12 {
		return ErrInvalidINN
	}
	for _, c := range inn {
		if c < '0' || c > '9' {
			return ErrInvalidINN
		}
	}
	return nil
}

// ValidateOGRN: 13 или 15 цифр.
func ValidateOGRN(ogrn string) error {
	if len(ogrn) != 13 && len(ogrn) != 15 {
		return ErrInvalidOGRN
	}
	for _, c := range ogrn {
		if c < '0' || c > '9' {
			return ErrInvalidOGRN
		}
	}
	return nil
}
