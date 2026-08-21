package imports

// ReportEntity is the created/reused split for one kind of row. The split is
// the only signal the operator has that a re-import was a no-op, so it is
// reported per entity rather than as one total.
type ReportEntity struct {
	Created int `json:"created"`
	Reused  int `json:"reused"`
}

// Report is what a successful import returns. Committed is false for a dry
// run; the counts are identical either way, because the dry run walks the same
// resolution and the same existence lookups as the real pass.
type Report struct {
	Committed   bool         `json:"committed"`
	Classes     ReportEntity `json:"classes"`
	Schedules   ReportEntity `json:"schedules"`
	Contacts    ReportEntity `json:"contacts"`
	Students    ReportEntity `json:"students"`
	Enrollments ReportEntity `json:"enrollments"`
}

// ErrorsPayload is the details half of a 422: the full list of row defects,
// so the operator fixes a whole workbook in one pass. Truncated says the list
// was capped; the count is the number omitted.
type ErrorsPayload struct {
	Errors    []RowError `json:"errors"`
	Truncated int        `json:"truncated,omitempty"`
}
