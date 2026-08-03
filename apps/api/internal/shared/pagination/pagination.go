// Package pagination parses and applies list-endpoint query parameters:
// page, per_page, and a whitelisted sort column.
package pagination

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"teka/apps/api/internal/shared/response"
)

const (
	defaultPerPage = 20
	maxPerPage     = 100
	// maxPage bounds the page number so Offset can never overflow int into a
	// negative value (which GORM silently drops, serving page 1 instead).
	maxPage = 1_000_000
)

// Params is the parsed, validated pagination/sort state for one request.
type Params struct {
	Page    int
	PerPage int
	// sortColumn is validated against the feature's whitelist; sortDesc
	// mirrors a leading "-" in the sort query parameter.
	sortColumn string
	sortDesc   bool
}

// Parse reads page/per_page/sort from the query string. allowedSorts maps the
// public sort key to the SQL column (e.g. "created_at" -> "created_at").
// Unknown sort keys fall back to defaultSort; bounds are clamped, not errors.
func Parse(c *gin.Context, defaultSort string, allowedSorts map[string]string) Params {
	p := Params{Page: 1, PerPage: defaultPerPage}

	if v, err := strconv.Atoi(c.Query("page")); err == nil && v > 0 {
		p.Page = min(v, maxPage)
	}
	if v, err := strconv.Atoi(c.Query("per_page")); err == nil && v > 0 {
		p.PerPage = min(v, maxPerPage)
	}

	sort := c.Query("sort")
	if sort == "" {
		sort = defaultSort
	}
	key := strings.TrimPrefix(sort, "-")
	col, ok := allowedSorts[key]
	if !ok {
		key = strings.TrimPrefix(defaultSort, "-")
		col = allowedSorts[key]
		sort = defaultSort
	}
	p.sortColumn = col
	p.sortDesc = strings.HasPrefix(sort, "-")
	return p
}

// Offset returns the SQL offset for the current page.
func (p Params) Offset() int { return (p.Page - 1) * p.PerPage }

// Scope applies limit/offset/order; use with db.Scopes(params.Scope).
func (p Params) Scope(db *gorm.DB) *gorm.DB {
	db = db.Limit(p.PerPage).Offset(p.Offset())
	if p.sortColumn != "" {
		dir := " ASC"
		if p.sortDesc {
			dir = " DESC"
		}
		db = db.Order(p.sortColumn + dir)
	}
	return db
}

// Meta builds the envelope meta block from a total row count.
func (p Params) Meta(total int64) response.Meta {
	pages := int(total) / p.PerPage
	if int(total)%p.PerPage != 0 {
		pages++
	}
	return response.Meta{Page: p.Page, PerPage: p.PerPage, Total: total, TotalPages: pages}
}
