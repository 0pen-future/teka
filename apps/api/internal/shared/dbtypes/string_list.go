// Package dbtypes holds column mappers shared across features so a JSONB
// shape is scanned and written one way everywhere.
package dbtypes

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// StringList maps an ordered JSONB string array column (curriculum lesson
// titles, plan activities, class tags). The whole list is always replaced at
// once — there is no per-element addressing anywhere in the product.
type StringList []string

// Value marshals the list, writing a nil slice as [] so JSONB columns shaped
// NOT NULL DEFAULT '[]' never see a SQL NULL.
func (l StringList) Value() (driver.Value, error) {
	if l == nil {
		l = StringList{}
	}
	return json.Marshal(l)
}

// Scan accepts the []byte/string forms the pgx/gorm stack hands over.
func (l *StringList) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		*l = nil
		return nil
	case []byte:
		return json.Unmarshal(v, l)
	case string:
		return json.Unmarshal([]byte(v), l)
	default:
		return fmt.Errorf("cannot scan %T into StringList", value)
	}
}
