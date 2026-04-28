package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGoose_BothMarkers(t *testing.T) {
	t.Parallel()
	content := `-- header comment
-- +goose Up
CREATE TABLE foo (id INT);
INSERT INTO foo VALUES (1);

-- +goose Down
DROP TABLE foo;
`
	up, down, err := parseGoose(content)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(up, "CREATE TABLE foo") || !strings.Contains(up, "INSERT INTO foo") {
		t.Errorf("up missing expected SQL: %q", up)
	}
	if strings.Contains(up, "DROP TABLE") {
		t.Errorf("up should NOT contain Down SQL: %q", up)
	}
	if !strings.Contains(down, "DROP TABLE foo") {
		t.Errorf("down missing expected SQL: %q", down)
	}
}

func TestParseGoose_OnlyUp(t *testing.T) {
	t.Parallel()
	content := `-- +goose Up
CREATE TABLE foo (id INT);
`
	up, down, err := parseGoose(content)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(up, "CREATE TABLE foo") {
		t.Errorf("up SQL missing: %q", up)
	}
	if down != "" {
		t.Errorf("expected empty down, got %q", down)
	}
}

func TestParseGoose_NoUpMarker(t *testing.T) {
	t.Parallel()
	if _, _, err := parseGoose("CREATE TABLE foo (id INT);"); err == nil {
		t.Fatal("expected error for missing marker")
	}
}

func TestDiscover_OrdersByVersionAndIgnoresUnrelated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Файлы в произвольном порядке + посторонние артефакты.
	files := map[string]string{
		"010_third.sql":      "-- +goose Up\nSELECT 1;\n",
		"001_first.sql":      "-- +goose Up\nSELECT 1;\n",
		"002_second.sql":     "-- +goose Up\nSELECT 2;\n",
		"README.md":          "# README\n",
		"not_a_migration.sql": "SELECT 1;\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Поддиректория — должна игнорироваться.
	if err := os.Mkdir(filepath.Join(dir, "tenant"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tenant", "001_x.sql"),
		[]byte("-- +goose Up\nSELECT 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	migs, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) != 3 {
		t.Fatalf("expected 3 migrations, got %d (%v)", len(migs), migs)
	}
	wantVersions := []int{1, 2, 10}
	for i, want := range wantVersions {
		if migs[i].Version != want {
			t.Errorf("migs[%d].Version = %d, want %d", i, migs[i].Version, want)
		}
	}
	if migs[0].Name != "first" {
		t.Errorf("name parse: %q, want 'first'", migs[0].Name)
	}
	if len(migs[0].Checksum) != 64 {
		t.Errorf("expected SHA-256 hex (64 chars), got %d chars", len(migs[0].Checksum))
	}
}

func TestDiscover_DuplicateVersionFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"001_a.sql", "001_b.sql"} {
		if err := os.WriteFile(filepath.Join(dir, name),
			[]byte("-- +goose Up\nSELECT 1;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Discover(dir); err == nil {
		t.Fatal("expected error for duplicate version")
	}
}

func TestDiscover_OnRealTenantTemplate(t *testing.T) {
	t.Parallel()
	// Smoke test против настоящего файла-шаблона tenant-миграции.
	// Защищает от регрессий при изменении формата файла.
	dir := filepath.Join("..", "..", "..", "..",
		"services", "tenant-service", "migrations", "tenant")
	migs, err := Discover(dir)
	if err != nil {
		t.Skipf("real tenant migrations not available: %v", err)
	}
	if len(migs) == 0 {
		t.Skip("no tenant migrations to validate")
	}
	for _, m := range migs {
		if m.UpSQL == "" {
			t.Errorf("%s has empty UpSQL", m.SourcePath)
		}
	}
}
