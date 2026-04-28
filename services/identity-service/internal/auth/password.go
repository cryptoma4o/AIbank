package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// BcryptCost — рабочий фактор для всех новых хешей.
// 12 — компромисс между ~250ms на современном железе и устойчивостью к bruteforce.
const BcryptCost = 12

// ErrInvalidPassword — пара login/password не сходится. Унифицирован, чтобы
// не давать атакующему различать "нет пользователя" от "неверный пароль".
var ErrInvalidPassword = errors.New("invalid credentials")

// Hash возвращает bcrypt-хеш пароля. Пароль должен быть не короче 8 символов
// и не длиннее 72 (предел bcrypt — он молча обрежет хвост, что небезопасно).
func Hash(password string) (string, error) {
	if len(password) < 8 {
		return "", fmt.Errorf("password: must be at least 8 chars")
	}
	if len(password) > 72 {
		return "", fmt.Errorf("password: must be at most 72 chars (bcrypt limit)")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return "", fmt.Errorf("password: hash: %w", err)
	}
	return string(h), nil
}

// Verify проверяет пароль против хеша. Возвращает ErrInvalidPassword при
// несовпадении или повреждённом хеше — оба случая снаружи неотличимы.
func Verify(hash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return ErrInvalidPassword
	}
	return nil
}
