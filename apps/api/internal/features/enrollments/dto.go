package enrollments

import (
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
)

// dateLayout is the wire form of every DATE field in this package.
const dateLayout = "2006-01-02"

// CreateRequest enrolls a student in a class. unit_price is deliberately
// absent: it is copied server-side from classes.default_unit_price, so no
// request path can set a bespoke rate — that is the enforcement of PRD
// section 4's single V1 pricing model, not a validation rule that could be
// relaxed by accident.
type CreateRequest struct {
	StudentID uuid.UUID `json:"student_id" binding:"required"`
	ClassID   uuid.UUID `json:"class_id" binding:"required"`
	// StartedOn defaults to today when absent. It is stored exactly as
	// given — never snapped to a month boundary or the class start date.
	StartedOn string `json:"started_on" binding:"omitempty,datetime=2006-01-02"`
}

// EndRequest closes an enrollment. EndedOn defaults to today when absent.
type EndRequest struct {
	EndedOn string `json:"ended_on" binding:"omitempty,datetime=2006-01-02"`
}

// EnrollmentResponse is the public enrollment shape; unit_price is an integer
// number of đồng, never a decimal.
type EnrollmentResponse struct {
	ID          uuid.UUID `json:"id"`
	StudentID   uuid.UUID `json:"student_id"`
	StudentName string    `json:"student_name"`
	ClassID     uuid.UUID `json:"class_id"`
	ClassName   string    `json:"class_name"`
	StartedOn   string    `json:"started_on"`
	EndedOn     *string   `json:"ended_on"`
	UnitPrice   int64     `json:"unit_price"`
	CreatedAt   time.Time `json:"created_at"`
}

// PickerStudent is one row of the enrollable-student picker: deliberately
// names only. The picker serves teachers who may not see contact data, so the
// shape itself guarantees no phone or contact id can leak through it.
type PickerStudent struct {
	ID       uuid.UUID `json:"id"`
	FullName string    `json:"full_name"`
}

// FromRow maps a joined enrollment row onto the response DTO.
func FromRow(row *Row) EnrollmentResponse {
	return EnrollmentResponse{
		ID:          row.ID,
		StudentID:   row.StudentID,
		StudentName: row.StudentName,
		ClassID:     row.ClassID,
		ClassName:   row.ClassName,
		StartedOn:   row.StartedOn.Format(dateLayout),
		EndedOn:     formatDatePtr(row.EndedOn),
		UnitPrice:   row.UnitPrice,
		CreatedAt:   row.CreatedAt,
	}
}

// parseDate converts a binding-validated YYYY-MM-DD string; a parse failure
// can only mean the value bypassed binding (service called directly), so it
// surfaces as a validation error rather than a 500.
func parseDate(field, value string) (time.Time, error) {
	t, err := time.Parse(dateLayout, value)
	if err != nil {
		return time.Time{}, apperror.Invalid("validation failed",
			map[string]string{field: "must be a date in YYYY-MM-DD form"})
	}
	return t, nil
}

func formatDatePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(dateLayout)
	return &s
}
