package centers

import (
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/id"
	"teka/apps/api/pkg/kanban"
)

// defaultColumnSpecs is the one Vietnamese literal for a new center's starter
// board: pkg/kanban carries no language of its own (see its README's
// localization boundary), so an adapter supplies the names. Order and
// is_done must match migration 000022's "-- teka:default-columns" block —
// that migration's own parity test freezes its SQL literal independently;
// this is the Go-side half of the same three columns.
var defaultColumnSpecs = []kanban.DefaultColumnSpec{
	{Name: "Cần làm", IsDone: false},
	{Name: "Đang làm", IsDone: false},
	{Name: "Hoàn thành", IsDone: true},
}

// DefaultColumns builds the three starter columns for a newly created
// center. CreatedAt/UpdatedAt are left zero: the INSERT lets the table
// defaults fill them. Exported so raw-SQL fixtures that create centers
// outside CreateCenter (the dev seeder) keep the same board invariant.
func DefaultColumns(centerID uuid.UUID) []kanban.Column {
	cols := kanban.DefaultColumns(id.New, defaultColumnSpecs)
	for i := range cols {
		cols[i].TenantID = kanban.TenantID(centerID)
	}
	return cols
}
