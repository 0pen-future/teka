package seeds

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/classinvites"
	"teka/apps/api/internal/features/classprogram"
	"teka/apps/api/internal/features/classstaff"
	"teka/apps/api/internal/features/courses"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/handoff"
	"teka/apps/api/internal/features/imports"
	"teka/apps/api/internal/features/library"
	"teka/apps/api/internal/features/paths"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/features/teaching"
	"teka/apps/api/internal/shared/authctx"
)

// The Giảng dạy (teaching) menu demo data uses a dedicated member so it never
// touches the invitations, roles, or class staff the other specs assert
// about Cô Lan, Thầy Minh, or Cô Thu. class-invitations.spec.ts, for one,
// cancels every pending/accepted invitation for Thầy Minh on "Văn 9 - Sáng
// Thứ Bảy"; a shared actor would race that cleanup.
var seedDemoTeacher = seedTeacher{Phone: "+84901000004", Password: "khoa-password", FullName: "Thầy Khoa"}

// demoTeacherGrantKeys are the three optIn permissions the phase asks the
// demo teacher to hold. BuildPermSet expands each key's implied read
// permission automatically, so granting only these three is sufficient.
var demoTeacherGrantKeys = []string{
	authctx.PermLibraryEdit,
	authctx.PermCoursesEdit,
	authctx.PermPrepAssign,
}

const (
	templatePublishedCode = "MAU-TOAN8"
	templateDraftCode     = "MAU-VAN9"
	learningPathCode      = "LO-TRINH-CB"
	programClassName      = "Toán 8 - Tối Thứ Ba"
	pendingInviteClass    = "Toán 8 - Tối Thứ Ba"
	acceptedInviteClass   = "Lý 7 - Chiều Thứ Năm"
)

// classCourseLinks names, per existing seeded class, the course code it
// should carry — every class the earlier seed blocks create gets a course.
var classCourseLinks = map[string]string{
	"Toán 8 - Tối Thứ Ba":  "TOAN8-CB",
	"Văn 9 - Sáng Thứ Bảy": "VAN9-NC",
	"Lý 7 - Chiều Thứ Năm": "LY7-CB",
}

// teachingMenuServices wires every service the Giảng dạy menu seed drives,
// mirroring router.go's construction order so seeded writes flow through the
// same authorization and business-rule code paths as a real request.
type teachingMenuServices struct {
	classes      *classes.Service
	centers      *centers.Service
	library      *library.Service
	courses      *courses.Service
	paths        *paths.Service
	classProgram *classprogram.Service
	classInvites *classinvites.Service
}

func newTeachingMenuServices(db *gorm.DB) teachingMenuServices {
	txMgr := database.NewTxManager(db)
	classStaffRepo := classstaff.NewRepository(db)
	classesSvc := classes.NewService(classes.NewRepository(db), txMgr, classStaffRepo)
	centersSvc := centers.NewService(centers.NewRepository(db), txMgr, nil)
	classStaffSvc := classstaff.NewService(classStaffRepo, centersSvc)
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db), nil)
	sessionsSvc := sessions.NewService(sessions.NewRepository(db), classesSvc, teachersSvc, enrollmentsSvc)
	centerLocker := imports.NewLocker(db)
	handoffSvc := handoff.NewService(classesSvc, sessionsSvc, centersSvc, classStaffRepo, centerLocker, txMgr)
	classInvitesSvc := classinvites.NewService(
		classinvites.NewRepository(db), classesSvc, centersSvc, classStaffRepo, classStaffSvc, handoffSvc, txMgr,
	)
	librarySvc := library.NewService(library.NewRepository(db), txMgr)
	coursesSvc := courses.NewService(courses.NewRepository(db), txMgr)
	pathsSvc := paths.NewService(paths.NewRepository(db), txMgr)
	teachingSvc := teaching.NewService(teaching.NewRepository(db), classesSvc, sessionsSvc, enrollmentsSvc, txMgr)
	classProgramSvc := classprogram.NewService(classprogram.NewRepository(db), classesSvc, teachingSvc, librarySvc, txMgr)

	return teachingMenuServices{
		classes:      classesSvc,
		centers:      centersSvc,
		library:      librarySvc,
		courses:      coursesSvc,
		paths:        pathsSvc,
		classProgram: classProgramSvc,
		classInvites: classInvitesSvc,
	}
}

// seedTeachingMenu populates the Giảng dạy menu's demo data: two program
// templates (one published, one with an open draft mid-preparation), three
// courses, a three-stage learning path, the existing classes linked to their
// course, one class with the published program applied, a demo teacher
// granted the menu's three optIn permissions, and two invitations for that
// teacher (pending and accepted). Every write goes through the real feature
// services and is skipped when its natural key already exists, so reseeding
// a populated database changes nothing.
func seedTeachingMenu(ctx context.Context, db *gorm.DB, log *slog.Logger, ownerSc authctx.Scope, centerID uuid.UUID) error {
	demoID, err := ensureMember(ctx, db, log, seedDemoTeacher, centerID)
	if err != nil {
		return err
	}
	demoSc, err := scopeFor(ctx, db, demoID)
	if err != nil {
		return err
	}

	svcs := newTeachingMenuServices(db)

	publishedVersionID, err := seedProgramTemplates(ctx, db, log, svcs, ownerSc, demoID)
	if err != nil {
		return err
	}

	courseIDs, err := seedCourseCatalog(ctx, db, log, svcs, ownerSc, publishedVersionID)
	if err != nil {
		return err
	}

	if err := seedLearningPath(ctx, db, log, svcs, ownerSc, courseIDs); err != nil {
		return err
	}

	if err := seedClassCourseLinks(ctx, db, log, svcs, ownerSc, courseIDs); err != nil {
		return err
	}

	if err := seedClassProgram(ctx, db, log, svcs, ownerSc, publishedVersionID); err != nil {
		return err
	}

	if err := seedDemoTeacherGrants(ctx, log, svcs, ownerSc, demoID); err != nil {
		return err
	}

	return seedClassInvitations(ctx, db, log, svcs, ownerSc, demoSc, demoID)
}

// seedProgramTemplates ensures the published and draft templates exist and
// returns the published template's version id for the course catalog and
// class program steps below.
func seedProgramTemplates(
	ctx context.Context, db *gorm.DB, log *slog.Logger, svcs teachingMenuServices, sc authctx.Scope, demoTeacherID uuid.UUID,
) (uuid.UUID, error) {
	publishedVersionID, err := ensurePublishedTemplate(ctx, db, log, svcs, sc)
	if err != nil {
		return uuid.Nil, err
	}
	if err := ensureDraftTemplate(ctx, db, log, svcs, sc, demoTeacherID); err != nil {
		return uuid.Nil, err
	}
	return publishedVersionID, nil
}

func ensurePublishedTemplate(
	ctx context.Context, db *gorm.DB, log *slog.Logger, svcs teachingMenuServices, sc authctx.Scope,
) (uuid.UUID, error) {
	if versionID, ok, err := findTemplateVersionByCode(ctx, db, sc.CenterID, templatePublishedCode, library.StatusPublished); err != nil {
		return uuid.Nil, err
	} else if ok {
		log.Info("seed: template already published, skipping", "code", templatePublishedCode)
		return versionID, nil
	}

	subject, level := "Toán", "Lớp 8"
	lessonCount := 3
	tpl, err := svcs.library.CreateTemplate(ctx, sc, library.TemplateRequest{
		Code:        templatePublishedCode,
		Name:        "Giáo trình Toán 8 chuẩn",
		Subject:     &subject,
		Level:       &level,
		LessonCount: &lessonCount,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("seed: create template %s: %w", templatePublishedCode, err)
	}
	if tpl.DraftVersionID == nil {
		return uuid.Nil, fmt.Errorf("seed: template %s has no draft version", templatePublishedCode)
	}
	if _, err := svcs.library.Publish(ctx, sc, *tpl.DraftVersionID); err != nil {
		return uuid.Nil, fmt.Errorf("seed: publish template %s: %w", templatePublishedCode, err)
	}
	log.Info("seed: template published", "code", templatePublishedCode, "version_id", *tpl.DraftVersionID)
	return *tpl.DraftVersionID, nil
}

// ensureDraftTemplate creates a template whose draft is mid-preparation: one
// lesson done and assigned with a due date and a checklist, one lesson in
// review, and one lesson left at the default "todo" — the mixed prep board
// the phase asks for.
func ensureDraftTemplate(
	ctx context.Context, db *gorm.DB, log *slog.Logger, svcs teachingMenuServices, sc authctx.Scope, demoTeacherID uuid.UUID,
) error {
	var count int64
	err := db.WithContext(ctx).Raw(
		"SELECT count(*) FROM program_templates WHERE center_id = ? AND code = ? AND deleted_at IS NULL",
		sc.CenterID, templateDraftCode,
	).Scan(&count).Error
	if err != nil {
		return fmt.Errorf("seed: look up template %s: %w", templateDraftCode, err)
	}
	if count > 0 {
		log.Info("seed: template already exists, skipping", "code", templateDraftCode)
		return nil
	}

	subject, level := "Văn", "Lớp 9"
	lessonCount := 3
	tpl, err := svcs.library.CreateTemplate(ctx, sc, library.TemplateRequest{
		Code:        templateDraftCode,
		Name:        "Giáo trình Văn 9 nâng cao",
		Subject:     &subject,
		Level:       &level,
		LessonCount: &lessonCount,
	})
	if err != nil {
		return fmt.Errorf("seed: create template %s: %w", templateDraftCode, err)
	}
	if tpl.DraftVersionID == nil {
		return fmt.Errorf("seed: template %s has no draft version", templateDraftCode)
	}

	lessonList, err := svcs.library.ListLessons(ctx, sc, *tpl.DraftVersionID)
	if err != nil {
		return fmt.Errorf("seed: list lessons of %s: %w", templateDraftCode, err)
	}
	if len(lessonList) < 3 {
		return fmt.Errorf("seed: template %s expected 3 lessons, got %d", templateDraftCode, len(lessonList))
	}

	doing := library.PrepDoing
	if _, err := svcs.library.UpdateLessonPrep(ctx, sc, lessonList[0].ID, library.PrepRequest{
		PrepStatus: &doing,
		Checklist: &[]library.ChecklistItem{
			{Label: "Soạn slide", Done: true},
			{Label: "In phiếu bài tập"},
		},
	}); err != nil {
		return fmt.Errorf("seed: set prep for %s buổi 1: %w", templateDraftCode, err)
	}
	dueDate := "2026-10-15"
	if _, err := svcs.library.UpdateLessonAssignment(ctx, sc, lessonList[0].ID, library.AssignmentRequest{
		AssigneeID: &demoTeacherID,
		DueDate:    &dueDate,
	}); err != nil {
		return fmt.Errorf("seed: assign %s buổi 1: %w", templateDraftCode, err)
	}

	review := library.PrepReview
	if _, err := svcs.library.UpdateLessonPrep(ctx, sc, lessonList[1].ID, library.PrepRequest{
		PrepStatus: &review,
	}); err != nil {
		return fmt.Errorf("seed: set prep for %s buổi 2: %w", templateDraftCode, err)
	}
	// Buổi 3 stays at the default "todo" status a new lesson is created with.

	log.Info("seed: template drafted", "code", templateDraftCode, "version_id", *tpl.DraftVersionID)
	return nil
}

// findTemplateVersionByCode looks up the live version of the given status for
// the template with the given code, by raw SQL since no service call returns
// a version id from a template code alone.
func findTemplateVersionByCode(
	ctx context.Context, db *gorm.DB, centerID uuid.UUID, code, status string,
) (uuid.UUID, bool, error) {
	var ids []uuid.UUID
	err := db.WithContext(ctx).Raw(`
		SELECT v.id FROM program_template_versions v
		JOIN program_templates t ON t.id = v.template_id
		WHERE t.center_id = ? AND t.code = ? AND t.deleted_at IS NULL AND v.status = ?`,
		centerID, code, status,
	).Scan(&ids).Error
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("seed: look up template %s: %w", code, err)
	}
	if len(ids) == 0 {
		return uuid.Nil, false, nil
	}
	return ids[0], true, nil
}

// courseSeed is one course the catalog step ensures exists.
type courseSeed struct {
	Code            string
	Name            string
	Subject         string
	Level           string
	UnitPrice       int64
	DefaultTemplate *uuid.UUID
}

// seedCourseCatalog ensures the three demo courses exist and returns their
// ids keyed by code, for the learning path and class-link steps below.
func seedCourseCatalog(
	ctx context.Context, db *gorm.DB, log *slog.Logger, svcs teachingMenuServices, sc authctx.Scope, publishedVersionID uuid.UUID,
) (map[string]uuid.UUID, error) {
	specs := []courseSeed{
		{Code: "TOAN8-CB", Name: "Toán 8 cơ bản", Subject: "Toán", Level: "Lớp 8", UnitPrice: 150_000, DefaultTemplate: &publishedVersionID},
		{Code: "VAN9-NC", Name: "Văn 9 nâng cao", Subject: "Văn", Level: "Lớp 9", UnitPrice: 200_000},
		{Code: "LY7-CB", Name: "Lý 7 cơ bản", Subject: "Lý", Level: "Lớp 7", UnitPrice: 180_000},
	}
	ids := make(map[string]uuid.UUID, len(specs))
	for _, c := range specs {
		courseID, err := ensureCourse(ctx, db, log, svcs, sc, c)
		if err != nil {
			return nil, err
		}
		ids[c.Code] = courseID
	}
	return ids, nil
}

func ensureCourse(
	ctx context.Context, db *gorm.DB, log *slog.Logger, svcs teachingMenuServices, sc authctx.Scope, c courseSeed,
) (uuid.UUID, error) {
	var ids []uuid.UUID
	err := db.WithContext(ctx).Raw(
		"SELECT id FROM courses WHERE center_id = ? AND code = ? AND deleted_at IS NULL",
		sc.CenterID, c.Code,
	).Scan(&ids).Error
	if err != nil {
		return uuid.Nil, fmt.Errorf("seed: look up course %s: %w", c.Code, err)
	}
	if len(ids) > 0 {
		log.Info("seed: course already exists, skipping", "code", c.Code)
		return ids[0], nil
	}

	subject, level := c.Subject, c.Level
	resp, err := svcs.courses.Create(ctx, sc, courses.CourseRequest{
		Code:                     c.Code,
		Name:                     c.Name,
		Subject:                  &subject,
		Level:                    &level,
		Status:                   courses.StatusActive,
		DefaultTemplateVersionID: c.DefaultTemplate,
		DefaultUnitPrice:         c.UnitPrice,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("seed: create course %s: %w", c.Code, err)
	}
	log.Info("seed: course created", "code", c.Code)
	return resp.ID, nil
}

// seedLearningPath ensures the demo path with its three course-recommending
// stages exists.
func seedLearningPath(
	ctx context.Context, db *gorm.DB, log *slog.Logger, svcs teachingMenuServices, sc authctx.Scope, courseIDs map[string]uuid.UUID,
) error {
	var count int64
	err := db.WithContext(ctx).Raw(
		"SELECT count(*) FROM learning_paths WHERE center_id = ? AND code = ? AND deleted_at IS NULL",
		sc.CenterID, learningPathCode,
	).Scan(&count).Error
	if err != nil {
		return fmt.Errorf("seed: look up learning path %s: %w", learningPathCode, err)
	}
	if count > 0 {
		log.Info("seed: learning path already exists, skipping", "code", learningPathCode)
		return nil
	}

	path, err := svcs.paths.Create(ctx, sc, paths.PathRequest{
		Code:   learningPathCode,
		Name:   "Lộ trình Toán - Văn - Lý cơ bản",
		Status: paths.StatusActive,
	})
	if err != nil {
		return fmt.Errorf("seed: create learning path %s: %w", learningPathCode, err)
	}

	stages := []struct {
		Name       string
		CourseCode string
	}{
		{"Giai đoạn 1: Toán 8", "TOAN8-CB"},
		{"Giai đoạn 2: Văn 9", "VAN9-NC"},
		{"Giai đoạn 3: Lý 7", "LY7-CB"},
	}
	for _, st := range stages {
		updated, err := svcs.paths.CreateStage(ctx, sc, path.ID, paths.StageRequest{Name: st.Name})
		if err != nil {
			return fmt.Errorf("seed: add stage %s to %s: %w", st.Name, learningPathCode, err)
		}
		stageID := updated.Stages[len(updated.Stages)-1].ID
		courseID, ok := courseIDs[st.CourseCode]
		if !ok {
			return fmt.Errorf("seed: course %s missing for stage %s", st.CourseCode, st.Name)
		}
		if _, err := svcs.paths.SetStageCourses(ctx, sc, path.ID, stageID, paths.StageCoursesRequest{
			CourseIDs: []uuid.UUID{courseID},
		}); err != nil {
			return fmt.Errorf("seed: set courses for stage %s: %w", st.Name, err)
		}
	}
	log.Info("seed: learning path created", "code", learningPathCode, "stages", len(stages))
	return nil
}

// seedClassCourseLinks attaches each existing seeded class to its matching
// course, resending the class's own unchanged full-replace fields (name,
// start/end date, default unit price) alongside the new course id, since
// classes.Update does not nil-patch those.
func seedClassCourseLinks(
	ctx context.Context, db *gorm.DB, log *slog.Logger, svcs teachingMenuServices, sc authctx.Scope, courseIDs map[string]uuid.UUID,
) error {
	for className, courseCode := range classCourseLinks {
		courseID, ok := courseIDs[courseCode]
		if !ok {
			return fmt.Errorf("seed: course %s missing for class %s", courseCode, className)
		}
		if err := ensureClassCourse(ctx, db, log, svcs, sc, className, courseID); err != nil {
			return err
		}
	}
	return nil
}

// findClassByName resolves a class id by center and name via raw SQL,
// bypassing the classes service's teacher-anchored lookup: some target
// classes were created under a different teacher's scope than sc (e.g. "Lý 7
// - Chiều Thứ Năm" under Thầy Minh), and the owner scope's write-wide access
// still needs the id to Get/Update through the service afterwards.
func findClassByName(ctx context.Context, db *gorm.DB, centerID uuid.UUID, className string) (uuid.UUID, error) {
	var ids []uuid.UUID
	if err := db.WithContext(ctx).Raw(
		"SELECT id FROM classes WHERE center_id = ? AND name = ? AND deleted_at IS NULL",
		centerID, className,
	).Scan(&ids).Error; err != nil {
		return uuid.Nil, fmt.Errorf("seed: look up class %q: %w", className, err)
	}
	if len(ids) == 0 {
		return uuid.Nil, fmt.Errorf("seed: class %q not found", className)
	}
	return ids[0], nil
}

func ensureClassCourse(
	ctx context.Context, db *gorm.DB, log *slog.Logger, svcs teachingMenuServices, sc authctx.Scope, className string, courseID uuid.UUID,
) error {
	classID, err := findClassByName(ctx, db, sc.CenterID, className)
	if err != nil {
		return err
	}

	current, err := svcs.classes.Get(ctx, sc, classID)
	if err != nil {
		return fmt.Errorf("seed: read class %q: %w", className, err)
	}
	if current.CourseID != nil && *current.CourseID == courseID {
		log.Info("seed: class already linked to course, skipping", "class", className)
		return nil
	}

	endDate := ""
	if current.EndDate != nil {
		endDate = current.EndDate.Format("2006-01-02")
	}
	courseIDStr := courseID.String()
	if _, err := svcs.classes.Update(ctx, sc, classID, classes.UpdateClassRequest{
		Name:             current.Name,
		StartDate:        current.StartDate.Format("2006-01-02"),
		EndDate:          endDate,
		DefaultUnitPrice: &current.DefaultUnitPrice,
		CourseID:         &courseIDStr,
	}); err != nil {
		return fmt.Errorf("seed: link class %q to course: %w", className, err)
	}
	log.Info("seed: class linked to course", "class", className)
	return nil
}

// seedClassProgram applies the published template version to
// programClassName. classprogram.Apply upserts the curriculum and is itself
// idempotent for a matching version, so the pre-check here only avoids an
// unnecessary write and a duplicate log line on reseed.
func seedClassProgram(
	ctx context.Context, db *gorm.DB, log *slog.Logger, svcs teachingMenuServices, sc authctx.Scope, publishedVersionID uuid.UUID,
) error {
	classID, err := findClassByName(ctx, db, sc.CenterID, programClassName)
	if err != nil {
		return err
	}

	existing, err := svcs.classProgram.Get(ctx, sc, classID)
	if err != nil {
		return fmt.Errorf("seed: read program of %q: %w", programClassName, err)
	}
	if existing != nil && existing.TemplateVersionID == publishedVersionID {
		log.Info("seed: class program already applied, skipping", "class", programClassName)
		return nil
	}

	if _, err := svcs.classProgram.Apply(ctx, sc, classID, classprogram.ApplyRequest{
		TemplateVersionID: publishedVersionID,
	}); err != nil {
		return fmt.Errorf("seed: apply program to %q: %w", programClassName, err)
	}
	log.Info("seed: class program applied", "class", programClassName)
	return nil
}

// seedDemoTeacherGrants grants the demo teacher the menu's three optIn
// permissions through the real center permission mechanism.
// ReplaceMemberOverrides unconditionally rewrites the member's override row
// even when the CAS check is bypassed at version 0, so the read-before-write
// here is what keeps a reseed from touching an already-granted row.
func seedDemoTeacherGrants(
	ctx context.Context, log *slog.Logger, svcs teachingMenuServices, sc authctx.Scope, demoTeacherID uuid.UUID,
) error {
	perms, err := svcs.centers.Permissions(ctx, sc)
	if err != nil {
		return fmt.Errorf("seed: read center permissions: %w", err)
	}
	for _, m := range perms.Members {
		if m.TeacherID != demoTeacherID {
			continue
		}
		if hasAllKeys(m.Grants, demoTeacherGrantKeys) {
			log.Info("seed: demo teacher already granted, skipping", "teacher_id", demoTeacherID)
			return nil
		}
		break
	}

	if err := svcs.centers.ReplaceMemberOverrides(ctx, sc, demoTeacherID, centers.MemberOverridesRequest{
		Grants: demoTeacherGrantKeys,
	}); err != nil {
		return fmt.Errorf("seed: grant permissions to demo teacher: %w", err)
	}
	log.Info("seed: demo teacher granted", "teacher_id", demoTeacherID, "grants", demoTeacherGrantKeys)
	return nil
}

func hasAllKeys(have, want []string) bool {
	set := make(map[string]struct{}, len(have))
	for _, k := range have {
		set[k] = struct{}{}
	}
	for _, k := range want {
		if _, ok := set[k]; !ok {
			return false
		}
	}
	return true
}

// seedClassInvitations sends the demo teacher a pending invitation on one
// class and an accepted one on another, through the real classinvites
// service (Send, then Accept — never Confirm, which would additionally write
// a class_staff stint that this phase does not ask for).
func seedClassInvitations(
	ctx context.Context, db *gorm.DB, log *slog.Logger, svcs teachingMenuServices, ownerSc, demoSc authctx.Scope, demoTeacherID uuid.UUID,
) error {
	if err := ensureClassInvitation(ctx, db, log, svcs, ownerSc, demoSc, pendingInviteClass, demoTeacherID, false); err != nil {
		return err
	}
	return ensureClassInvitation(ctx, db, log, svcs, ownerSc, demoSc, acceptedInviteClass, demoTeacherID, true)
}

func ensureClassInvitation(
	ctx context.Context, db *gorm.DB, log *slog.Logger, svcs teachingMenuServices,
	ownerSc, demoSc authctx.Scope, className string, demoTeacherID uuid.UUID, accept bool,
) error {
	classID, err := findClassByName(ctx, db, ownerSc.CenterID, className)
	if err != nil {
		return err
	}

	var existing []struct {
		ID     uuid.UUID
		Status string
	}
	if err := db.WithContext(ctx).Raw(
		"SELECT id, status FROM class_invitations WHERE center_id = ? AND class_id = ? AND teacher_id = ? AND status IN ('pending', 'accepted')",
		ownerSc.CenterID, classID, demoTeacherID,
	).Scan(&existing).Error; err != nil {
		return fmt.Errorf("seed: look up invitation on %q: %w", className, err)
	}
	if len(existing) > 0 {
		log.Info("seed: class invitation already exists, skipping", "class", className, "status", existing[0].Status)
		return nil
	}

	message := "Nhờ thầy/cô hỗ trợ lớp này giúp trung tâm nhé."
	inv, err := svcs.classInvites.Send(ctx, ownerSc, classID, classinvites.SendRequest{
		TeacherID: demoTeacherID,
		RoleKey:   authctx.StaffRoleTroGiang,
		Message:   &message,
	})
	if err != nil {
		return fmt.Errorf("seed: send invitation on %q: %w", className, err)
	}
	if !accept {
		log.Info("seed: class invitation sent (pending)", "class", className)
		return nil
	}
	if _, err := svcs.classInvites.Accept(ctx, demoSc, inv.ID); err != nil {
		return fmt.Errorf("seed: accept invitation on %q: %w", className, err)
	}
	log.Info("seed: class invitation accepted", "class", className)
	return nil
}
