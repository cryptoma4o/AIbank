// Package format renders Tenant lists/objects as either a table or JSON.
//
// The table layout is column-aligned with header underlines; columns are
// kept narrow on purpose so that CI logs stay readable.
package format

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/aibank/platform/tools/tenant-cli/internal/client"
)

const (
	// FormatTable is the default human-friendly output.
	FormatTable = "table"
	// FormatJSON emits raw JSON (one tenant or array).
	FormatJSON = "json"
)

// Tenants renders a list of tenants in the requested format.
func Tenants(w io.Writer, tenants []client.Tenant, format string) error {
	switch format {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(tenants)
	case "", FormatTable:
		return renderTable(w, tenants)
	default:
		return fmt.Errorf("unknown format %q (want table|json)", format)
	}
}

// SingleTenant renders one tenant.  In table mode it shows a key/value list.
func SingleTenant(w io.Writer, t *client.Tenant, format string) error {
	switch format {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(t)
	case "", FormatTable:
		return renderKV(w, t)
	default:
		return fmt.Errorf("unknown format %q (want table|json)", format)
	}
}

// renderTable prints tenants as fixed-width columns.
func renderTable(w io.Writer, tenants []client.Tenant) error {
	headers := []string{"ID", "NAME", "BIK", "INN", "STATUS", "MODE", "CREATED"}
	rows := make([][]string, 0, len(tenants))
	for _, t := range tenants {
		rows = append(rows, []string{
			t.ID,
			truncate(t.Name, 28),
			t.BIK,
			t.INN,
			t.Status,
			t.DeploymentMode,
			t.CreatedAt.Format("2006-01-02"),
		})
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	if err := writeRow(w, headers, widths); err != nil {
		return err
	}
	sep := make([]string, len(headers))
	for i, wd := range widths {
		sep[i] = strings.Repeat("-", wd)
	}
	if err := writeRow(w, sep, widths); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeRow(w, row, widths); err != nil {
			return err
		}
	}
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "(no tenants)")
		return err
	}
	return nil
}

func renderKV(w io.Writer, t *client.Tenant) error {
	pairs := [][2]string{
		{"id", t.ID},
		{"name", t.Name},
		{"bik", t.BIK},
		{"inn", t.INN},
		{"status", t.Status},
		{"deployment_mode", t.DeploymentMode},
		{"created_at", t.CreatedAt.Format("2006-01-02T15:04:05Z07:00")},
		{"updated_at", t.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")},
	}
	keyWidth := 0
	for _, p := range pairs {
		if len(p[0]) > keyWidth {
			keyWidth = len(p[0])
		}
	}
	for _, p := range pairs {
		if _, err := fmt.Fprintf(w, "%-*s  %s\n", keyWidth, p[0], p[1]); err != nil {
			return err
		}
	}
	return nil
}

func writeRow(w io.Writer, cells []string, widths []int) error {
	parts := make([]string, len(cells))
	for i, c := range cells {
		parts[i] = padRight(c, widths[i])
	}
	_, err := fmt.Fprintln(w, strings.Join(parts, "  "))
	return err
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
