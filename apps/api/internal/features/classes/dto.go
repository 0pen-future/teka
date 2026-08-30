package classes

import (
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
)

// dateLayout is the wire form of every DATE field in this package.
const dateLayout = "2006-01-02"

// ScheduleRequest is one weekly timetable row in a create/add payload.
// Weekday is a pointer so 0 (Sunday) survives binding:"required"; a plain int
// would silently default a missing weekday to Sunday.
type ScheduleRequest struct {
	Weekday     *int16 `json:"weekday" binding:"required,min=0,max=6"`
	StartTime   string `json:"start_time" binding:"required,hhmm"`
	DurationMin int16  `json:"duration_min" binding:"required,min=1"`
	// EffectiveFrom defaults to the class start_date when absent.
	EffectiveFrom string `json:"effective_from" binding:"omitempty,datetime=2006-01-02"`
	EffectiveTo   string `json:"effective_to" binding:"omitempty,datetime=2006-01-02"`
}

// CreateClassRequest creates a class with its schedules atomically — a class
// with no timetable generates no sessions, so schedules are mandatory here.
// DefaultUnitPrice is a pointer so a legitimately free class (0 đồng, allowed
// by the CHECK) passes binding:"required".
type CreateClassRequest struct {
	Name             string            `json:"name" binding:"required,min=1,max=100"`
	StartDate        string            `json:"start_date" binding:"required,datetime=2006-01-02"`
	EndDate          string            `json:"end_date" binding:"omitempty,datetime=2006-01-02"`
	DefaultUnitPrice *int64            `json:"default_unit_price" binding:"required,min=0"`
	Schedules        []ScheduleRequest `json:"schedules" binding:"required,min=1,dive"`
}

// UpdateClassRequest edits the class's own fields; schedules are a
// sub-resource and status changes go through archive.
type UpdateClassRequest struct {
	Name             string `json:"name" binding:"required,min=1,max=100"`
	StartDate        string `json:"start_date" binding:"required,datetime=2006-01-02"`
	EndDate          string `json:"end_date" binding:"omitempty,datetime=2006-01-02"`
	DefaultUnitPrice *int64 `json:"default_unit_price" binding:"required,min=0"`
}

// UpdateScheduleRequest edits one schedule row in place. The intended use is
// correcting a mistyped row or closing it by setting effective_to; a real
// timetable change should close the old row and add a new one so past
// sessions stay explicable.
type UpdateScheduleRequest struct {
	Weekday       *int16 `json:"weekday" binding:"required,min=0,max=6"`
	StartTime     string `json:"start_time" binding:"required,hhmm"`
	DurationMin   int16  `json:"duration_min" binding:"required,min=1"`
	EffectiveFrom string `json:"effective_from" binding:"required,datetime=2006-01-02"`
	EffectiveTo   string `json:"effective_to" binding:"omitempty,datetime=2006-01-02"`
}

// ScheduleResponse is the public schedule shape.
type ScheduleResponse struct {
	ID            uuid.UUID `json:"id"`
	Weekday       int16     `json:"weekday"`
	StartTime     string    `json:"start_time"`
	DurationMin   int16     `json:"duration_min"`
	EffectiveFrom string    `json:"effective_from"`
	EffectiveTo   *string   `json:"effective_to"`
}

// ClassResponse is the public class shape; default_unit_price is an integer
// number of đồng, never a decimal.
type ClassResponse struct {
	ID               uuid.UUID          `json:"id"`
	Name             string             `json:"name"`
	TeacherID        uuid.UUID          `json:"teacher_id"`
	StartDate        string             `json:"start_date"`
	EndDate          *string            `json:"end_date"`
	DefaultUnitPrice int64              `json:"default_unit_price"`
	Status           string             `json:"status"`
	Schedules        []ScheduleResponse `json:"schedules"`
	// MyStaffRoles lists the CALLER's active class_staff role keys on this
	// class — per-caller data, so only the readable GET paths fill it (via
	// FromModelWithRoles); every other producer, the dashboard included,
	// leaves it empty.
	MyStaffRoles []string  `json:"my_staff_roles"`
	CreatedAt    time.Time `json:"created_at"`
}

// FromSchedule maps a schedule row onto the response DTO.
func FromSchedule(s *Schedule) ScheduleResponse {
	return ScheduleResponse{
		ID:            s.ID,
		Weekday:       s.Weekday,
		StartTime:     string(s.StartTime),
		DurationMin:   s.DurationMin,
		EffectiveFrom: s.EffectiveFrom.Format(dateLayout),
		EffectiveTo:   formatDatePtr(s.EffectiveTo),
	}
}

// FromModel maps a class (with whatever schedules are loaded) onto the
// response DTO.
func FromModel(class *Class) ClassResponse {
	schedules := make([]ScheduleResponse, 0, len(class.Schedules))
	for i := range class.Schedules {
		schedules = append(schedules, FromSchedule(&class.Schedules[i]))
	}
	return ClassResponse{
		ID:               class.ID,
		Name:             class.Name,
		TeacherID:        class.TeacherID,
		StartDate:        class.StartDate.Format(dateLayout),
		EndDate:          formatDatePtr(class.EndDate),
		DefaultUnitPrice: class.DefaultUnitPrice,
		Status:           class.Status,
		Schedules:        schedules,
		MyStaffRoles:     []string{},
		CreatedAt:        class.CreatedAt,
	}
}

// FromModelWithRoles is FromModel plus the caller's active staff roles. Kept
// separate so FromModel stays a pure mapper shared with per-teacher surfaces
// like the dashboard, which must never carry per-caller fields.
func FromModelWithRoles(class *Class, roles []string) ClassResponse {
	resp := FromModel(class)
	if len(roles) > 0 {
		resp.MyStaffRoles = roles
	}
	return resp
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

// parseDatePtr is parseDate for optional fields; "" means nil.
func parseDatePtr(field, value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	t, err := parseDate(field, value)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func formatDatePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(dateLayout)
	return &s
}
