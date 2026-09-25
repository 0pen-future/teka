package classes

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
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
	DurationMin int16  `json:"duration_min" binding:"required,min=1,max=600"`
	// EffectiveFrom defaults to the class start_date when absent.
	EffectiveFrom string `json:"effective_from" binding:"omitempty,datetime=2006-01-02"`
	EffectiveTo   string `json:"effective_to" binding:"omitempty,datetime=2006-01-02"`
}

// CreateClassRequest creates a class with its schedules atomically — a class
// with no timetable generates no sessions, so schedules are mandatory here.
// DefaultUnitPrice is a pointer so a legitimately free class (0 đồng, allowed
// by the CHECK) passes binding:"required".
type CreateClassRequest struct {
	Name      string `json:"name" binding:"required,min=1,max=100"`
	StartDate string `json:"start_date" binding:"required,datetime=2006-01-02"`
	EndDate   string `json:"end_date" binding:"omitempty,datetime=2006-01-02"`
	// DefaultUnitPrice may be left out only when course_id is set: the
	// class then copies the course's default price.
	DefaultUnitPrice *int64 `json:"default_unit_price" binding:"omitempty,min=0"`
	// Schedules is required for a scheduled class and must be empty for a
	// self_paced one; the service enforces that pairing since it depends on
	// StudyMode, not on binding tags alone.
	Schedules []ScheduleRequest `json:"schedules" binding:"omitempty,dive"`
	// CourseID attaches the class to a live course of the same center;
	// blank or absent means no course. No uuid tag: the validator treats a
	// pointer to "" as present and would fail it, so the service parses.
	CourseID *string `json:"course_id"`
	// Code is the display code (mã lớp); blank or absent means "mint one".
	// Its shape is checked by the service via classcode.Valid so the message
	// lands on this field either way.
	Code *string  `json:"code" binding:"omitempty,max=20"`
	Tags []string `json:"tags" binding:"omitempty,max=10,dive,max=30"`
	Note *string  `json:"note" binding:"omitempty,max=1000"`
	// Room is the physical/virtual room name; blank or absent means
	// unassigned.
	Room string `json:"room" binding:"omitempty,max=50"`
	// StudyMode defaults to "scheduled" when absent.
	StudyMode string `json:"study_mode" binding:"omitempty,oneof=scheduled self_paced"`
}

// UpdateClassRequest edits the class's own fields; schedules are a
// sub-resource and status changes go through archive. The original fields
// keep their full-replace contract; the catalog fields below are pointers
// where nil means "leave as stored", so a toggle can send only itself.
type UpdateClassRequest struct {
	Name             string    `json:"name" binding:"required,min=1,max=100"`
	StartDate        string    `json:"start_date" binding:"required,datetime=2006-01-02"`
	EndDate          string    `json:"end_date" binding:"omitempty,datetime=2006-01-02"`
	DefaultUnitPrice *int64    `json:"default_unit_price" binding:"required,min=0"`
	Code             *string   `json:"code" binding:"omitempty,max=20"`
	Tags             *[]string `json:"tags" binding:"omitempty,max=10,dive,max=30"`
	Recruiting       *bool     `json:"recruiting"`
	// Note replaces the stored note; an empty string clears it.
	Note *string `json:"note" binding:"omitempty,max=1000"`
	// CourseID follows the same patch rule: nil keeps the stored course, an
	// empty string detaches it, a uuid attaches that course. Untagged for
	// the same reason as on create: "" must reach the service.
	CourseID *string `json:"course_id"`
	// ParentClassID and LineageNote follow the same patch rule: nil keeps,
	// "" clears, a uuid links a live class of the same center (never the
	// class itself). Untagged for the same reason as course_id.
	ParentClassID *string `json:"parent_class_id"`
	LineageNote   *string `json:"lineage_note" binding:"omitempty,max=1000"`
	// Room follows the same patch rule: nil keeps, "" clears.
	Room *string `json:"room" binding:"omitempty,max=50"`
	// StudyMode follows the same patch rule: nil keeps the stored mode.
	// Untagged: an empty string is not a valid mode and must reach the
	// service to surface as a field error rather than being silently dropped
	// by a binding tag.
	StudyMode *string `json:"study_mode"`
	// NextClassID follows the same patch rule: nil keeps every existing
	// child link, "" unlinks every live child, a uuid links that live class
	// of the same center as the single child (see resolveNextClassLink).
	// Untagged for the same "" reason as course_id.
	NextClassID *string `json:"next_class_id"`
}

// CourseRefResponse is the course a class is attached to, as embedded in
// ClassResponse: enough for a chip that links to the catalog.
type CourseRefResponse struct {
	ID   uuid.UUID `json:"id"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// ClassStatsResponse counts the classes the caller can read, bucketed by the
// same phase rule the list filter applies; All is the total across buckets
// and Recruiting counts the classes open for recruitment (flag on, not ended
// or archived), the same set the list's recruiting filter returns.
type ClassStatsResponse struct {
	All        int64 `json:"all"`
	Upcoming   int64 `json:"upcoming"`
	Running    int64 `json:"running"`
	Ended      int64 `json:"ended"`
	Archived   int64 `json:"archived"`
	Recruiting int64 `json:"recruiting"`
}

// UpdateScheduleRequest edits one schedule row in place. The intended use is
// correcting a mistyped row or closing it by setting effective_to; a real
// timetable change should close the old row and add a new one so past
// sessions stay explicable.
type UpdateScheduleRequest struct {
	Weekday       *int16 `json:"weekday" binding:"required,min=0,max=6"`
	StartTime     string `json:"start_time" binding:"required,hhmm"`
	DurationMin   int16  `json:"duration_min" binding:"required,min=1,max=600"`
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
	ID               uuid.UUID `json:"id"`
	Name             string    `json:"name"`
	TeacherID        uuid.UUID `json:"teacher_id"`
	StartDate        string    `json:"start_date"`
	EndDate          *string   `json:"end_date"`
	DefaultUnitPrice int64     `json:"default_unit_price"`
	Status           string    `json:"status"`
	Code             string    `json:"code"`
	Tags             []string  `json:"tags"`
	Recruiting       bool      `json:"recruiting"`
	Note             *string   `json:"note"`
	// Phase is derived from status and the dates against today (see
	// PhaseOf); the client renders it and never recomputes it.
	Phase     string             `json:"phase"`
	Schedules []ScheduleResponse `json:"schedules"`
	// MyStaffRoles lists the CALLER's active class_staff role keys on this
	// class — per-caller data, so only the readable GET paths fill it (via
	// FromModelWithRoles); every other producer, the dashboard included,
	// leaves it empty.
	MyStaffRoles []string `json:"my_staff_roles"`
	// StudentCount is the number of enrollments still open on the class
	// (ended_on IS NULL, not deleted) — the same predicate GET /enrollments
	// applies for active=true, so a picker showing this count matches the
	// rows that endpoint lists. Like MyStaffRoles it is filled only by the
	// readable GET paths; every other producer leaves it 0.
	StudentCount int `json:"student_count"`
	// Course is the attached course, null when the class has none.
	Course *CourseRefResponse `json:"course"`
	// ParentClassID and LineageNote describe the class's place in its
	// lineage (lịch sử lớp); both null when the class stands alone.
	ParentClassID *uuid.UUID `json:"parent_class_id"`
	LineageNote   *string    `json:"lineage_note"`
	// Room and StudyMode are the wizard's "Hình thức học" fields.
	Room      string `json:"room"`
	StudyMode string `json:"study_mode"`
	// NextClassID is the earliest-created live child whose parent_class_id
	// is this class (lớp kế tiếp); null when it has none.
	NextClassID *uuid.UUID `json:"next_class_id"`
	CreatedAt   time.Time  `json:"created_at"`
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
	tags := make([]string, 0, len(class.Tags))
	tags = append(tags, class.Tags...)
	return ClassResponse{
		ID:               class.ID,
		Name:             class.Name,
		TeacherID:        class.TeacherID,
		StartDate:        class.StartDate.Format(dateLayout),
		EndDate:          formatDatePtr(class.EndDate),
		DefaultUnitPrice: class.DefaultUnitPrice,
		Status:           class.Status,
		Code:             class.Code,
		Tags:             tags,
		Recruiting:       class.Recruiting,
		Note:             class.Note,
		Phase:            PhaseOf(class, Today()),
		Schedules:        schedules,
		MyStaffRoles:     []string{},
		Course:           courseRef(class.Course),
		ParentClassID:    class.ParentClassID,
		LineageNote:      class.LineageNote,
		Room:             class.Room,
		StudyMode:        class.StudyMode,
		NextClassID:      class.NextClassID,
		CreatedAt:        class.CreatedAt,
	}
}

func courseRef(ref *CourseRef) *CourseRefResponse {
	if ref == nil {
		return nil
	}
	return &CourseRefResponse{ID: ref.ID, Code: ref.Code, Name: ref.Name}
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

// validateDateRange rejects an end_date earlier than start_date; the same
// day for both is allowed (a class that starts and ends on one day).
func validateDateRange(startDate time.Time, endDate *time.Time) error {
	if endDate != nil && endDate.Before(startDate) {
		return apperror.Invalid("validation failed",
			map[string]string{"end_date": "must not be earlier than start_date"})
	}
	return nil
}

func formatDatePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(dateLayout)
	return &s
}

// availabilitySlotPattern matches one "slot" query value: "<weekday
// 0-6>-<HH:MM>-<duration minutes>", e.g. "1-08:00-90".
var availabilitySlotPattern = regexp.MustCompile(`^([0-6])-([01]\d|2[0-3]):([0-5]\d)-(\d+)$`)

// AvailabilitySlot is one parsed "slot" query value the availability endpoint
// checks classes against.
type AvailabilitySlot struct {
	Weekday     int16
	StartTime   TimeOfDay
	DurationMin int16
}

// parseAvailabilitySlot parses one raw "slot" query value; the returned error
// is nil only on success.
func parseAvailabilitySlot(raw string) (AvailabilitySlot, error) {
	m := availabilitySlotPattern.FindStringSubmatch(raw)
	if m == nil {
		return AvailabilitySlot{}, fmt.Errorf("malformed slot %q", raw)
	}
	weekday, err := strconv.ParseInt(m[1], 10, 16)
	if err != nil {
		return AvailabilitySlot{}, err
	}
	dur, err := strconv.ParseInt(m[4], 10, 16)
	if err != nil || dur < 1 {
		return AvailabilitySlot{}, fmt.Errorf("bad duration in slot %q", raw)
	}
	return AvailabilitySlot{
		Weekday:     int16(weekday),
		StartTime:   TimeOfDay(m[2] + ":" + m[3]),
		DurationMin: int16(dur),
	}, nil
}

// ParseAvailabilitySlots parses every raw "slot" query value; it returns a
// field error naming the "slot" key on the first malformed value.
func ParseAvailabilitySlots(raw []string) ([]AvailabilitySlot, error) {
	slots := make([]AvailabilitySlot, 0, len(raw))
	for _, r := range raw {
		slot, err := parseAvailabilitySlot(strings.TrimSpace(r))
		if err != nil {
			return nil, apperror.Invalid("validation failed",
				map[string]string{"slot": "must be <weekday 0-6>-<HH:MM>-<duration minutes>"})
		}
		slots = append(slots, slot)
	}
	return slots, nil
}

// AvailabilityRoomResponse is one room's busy/free state for the requested
// slots.
type AvailabilityRoomResponse struct {
	Name string `json:"name"`
	Free bool   `json:"free"`
}

// AvailabilityTeacherResponse is one center member's busy/free state for the
// requested slots.
type AvailabilityTeacherResponse struct {
	TeacherID uuid.UUID `json:"teacher_id"`
	Name      string    `json:"name"`
	Free      bool      `json:"free"`
}

// AvailabilityResponse is the public shape of GET /classes/availability.
type AvailabilityResponse struct {
	Rooms    []AvailabilityRoomResponse    `json:"rooms"`
	Teachers []AvailabilityTeacherResponse `json:"teachers"`
}
