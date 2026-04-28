// Package source находит и парсит SQL-миграции в формате goose
// (`-- +goose Up` / `-- +goose Down`).
//
// Используется как platform, так и tenant runner-ами для дискавери файлов
// и вычисления контрольных сумм (см. ADR-0005).
package source

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Migration — одна SQL-миграция.
type Migration struct {
	Version    int
	Name       string
	UpSQL      string
	DownSQL    string
	Checksum   string // SHA-256 содержимого файла, фиксирует drift
	SourcePath string
}

var fileNameRE = regexp.MustCompile(`^(\d+)_([a-z0-9_]+)\.sql$`)

// Discover читает каталог и возвращает миграции в порядке возрастания версий.
// Подкаталоги игнорируются. Файлы, не подходящие под `NNN_name.sql`, игнорируются.
// Дубликаты версий — ошибка.
func Discover(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}

	var migs []Migration
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		match := fileNameRE.FindStringSubmatch(e.Name())
		if match == nil {
			continue
		}
		version, _ := strconv.Atoi(match[1])
		path := filepath.Join(dir, e.Name())

		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		up, down, err := parseGoose(string(content))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		sum := sha256.Sum256(content)
		migs = append(migs, Migration{
			Version:    version,
			Name:       match[2],
			UpSQL:      up,
			DownSQL:    down,
			Checksum:   hex.EncodeToString(sum[:]),
			SourcePath: path,
		})
	}

	sort.Slice(migs, func(i, j int) bool { return migs[i].Version < migs[j].Version })

	for i := 1; i < len(migs); i++ {
		if migs[i].Version == migs[i-1].Version {
			return nil, fmt.Errorf("duplicate version %d (%s and %s)",
				migs[i].Version, migs[i-1].SourcePath, migs[i].SourcePath)
		}
	}
	return migs, nil
}

func parseGoose(content string) (up, down string, err error) {
	const upMarker = "-- +goose Up"
	const downMarker = "-- +goose Down"

	upIdx := strings.Index(content, upMarker)
	if upIdx < 0 {
		return "", "", fmt.Errorf("missing %q marker", upMarker)
	}
	rest := content[upIdx+len(upMarker):]
	downIdx := strings.Index(rest, downMarker)
	if downIdx < 0 {
		return strings.TrimSpace(rest), "", nil
	}
	up = strings.TrimSpace(rest[:downIdx])
	down = strings.TrimSpace(rest[downIdx+len(downMarker):])
	return up, down, nil
}
