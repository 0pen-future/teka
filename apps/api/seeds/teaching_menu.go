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
// templates (one published, one with an open draft mid-preparation), a
// small library bank (materials and exercises), the v5 draft content on top
// of it (an exercise group with one exercise assigned, a self_study lesson,
// a two-group score set and a student log field), three courses, a
// three-stage learning path, the existing classes linked to their course,
// one class with the published program applied, a demo teacher granted the
// menu's three optIn permissions, and two invitations for that teacher
// (pending and accepted). Every write goes through the real feature
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

	exerciseIDs, err := seedLibraryBank(ctx, db, log, svcs, ownerSc)
	if err != nil {
		return err
	}

	publishedVersionID, err := seedProgramTemplates(ctx, db, log, svcs, ownerSc, demoID, exerciseIDs)
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
// class program steps below. exerciseIDs is the library bank's seeded
// exercises, used to fill the draft template's exercise group.
func seedProgramTemplates(
	ctx context.Context, db *gorm.DB, log *slog.Logger, svcs teachingMenuServices, sc authctx.Scope, demoTeacherID uuid.UUID, exerciseIDs []uuid.UUID,
) (uuid.UUID, error) {
	publishedVersionID, err := ensurePublishedTemplate(ctx, db, log, svcs, sc)
	if err != nil {
		return uuid.Nil, err
	}
	if err := ensureDraftTemplate(ctx, db, log, svcs, sc, demoTeacherID, exerciseIDs); err != nil {
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

// draftExerciseGroupName names the one exercise group the v5 draft content
// seeds on MAU-VAN9's draft version.
const draftExerciseGroupName = "Khởi động"

// draftSelfStudyUnit is the unit label of the self_study lesson the v5
// draft content adds to MAU-VAN9's draft version.
const draftSelfStudyUnit = "Unit 1"

// ensureDraftTemplate creates a template whose draft is mid-preparation: one
// lesson done and assigned with a due date and a checklist, one lesson in
// review, and one lesson left at the default "todo" — the mixed prep board
// the phase asks for. It also seeds the v5 draft content: one exercise
// group carrying one bank exercise, a fourth lesson in self_study mode with
// unit "Unit 1" the exercise is assigned into, a two-group score set and a
// student-kind log field. exerciseIDs must hold at least one id (the
// library bank the caller seeds first).
func ensureDraftTemplate(
	ctx context.Context, db *gorm.DB, log *slog.Logger, svcs teachingMenuServices, sc authctx.Scope, demoTeacherID uuid.UUID, exerciseIDs []uuid.UUID,
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

	// One transaction for every v5 write: ensureDraftTemplate's natural-key
	// guard above only checks the template itself, so a failure partway
	// through the group/lesson/score-set/log-field chain would otherwise
	// leave the template seeded but its v5 content half-written, and a
	// reseed would then skip it forever (the guard sees the template and
	// stops looking).
	if err := database.NewTxManager(db).WithinTx(ctx, func(ctx context.Context) error {
		return seedDraftTemplateV5Content(ctx, svcs, sc, *tpl.DraftVersionID, exerciseIDs)
	}); err != nil {
		return err
	}

	log.Info("seed: template drafted", "code", templateDraftCode, "version_id", *tpl.DraftVersionID)
	return nil
}

// seedDraftTemplateV5Content adds the v5 fields to a freshly created draft
// version: one exercise group, a fourth self_study lesson carrying the
// group's one exercise, a two-group score set and a student log field. It
// runs only once, from ensureDraftTemplate's own natural-key guard, so it
// needs no find-then-skip of its own.
func seedDraftTemplateV5Content(
	ctx context.Context, svcs teachingMenuServices, sc authctx.Scope, draftVersionID uuid.UUID, exerciseIDs []uuid.UUID,
) error {
	if len(exerciseIDs) == 0 {
		return fmt.Errorf("seed: no library exercises available to assign to %s", templateDraftCode)
	}

	group, err := svcs.library.CreateExerciseGroup(ctx, sc, draftVersionID, library.ExerciseGroupRequest{
		Name: draftExerciseGroupName,
	})
	if err != nil {
		return fmt.Errorf("seed: create exercise group for %s: %w", templateDraftCode, err)
	}

	unit := draftSelfStudyUnit
	selfStudyLesson, err := svcs.library.CreateLesson(ctx, sc, draftVersionID, library.LessonRequest{
		Title: "Buổi tự học Unit 1",
		Mode:  library.LessonModeSelfStudy,
		Unit:  &unit,
	})
	if err != nil {
		return fmt.Errorf("seed: create self-study lesson for %s: %w", templateDraftCode, err)
	}

	groupID := group.ID
	if _, err := svcs.library.SetLessonExercises(ctx, sc, selfStudyLesson.ID, []library.LessonExerciseInput{
		{ExerciseID: exerciseIDs[0], GroupID: &groupID},
	}); err != nil {
		return fmt.Errorf("seed: assign exercise to group on %s: %w", templateDraftCode, err)
	}

	if _, err := svcs.library.SetScoreSet(ctx, sc, draftVersionID, []library.ScoreSetGroupInput{
		{
			Key:   "giua_ky",
			Title: "Giữa kỳ",
			Components: []library.ScoreComponentInput{
				{Key: "kiem_tra_15p", Label: "Kiểm tra 15 phút", Max: 10, Weight: 1},
			},
		},
		{
			Key:   "cuoi_ky",
			Title: "Cuối kỳ",
			Components: []library.ScoreComponentInput{
				{Key: "bai_thi", Label: "Bài thi cuối kỳ", Max: 10, Weight: 2},
			},
		},
	}); err != nil {
		return fmt.Errorf("seed: set score set for %s: %w", templateDraftCode, err)
	}

	if _, err := svcs.library.SetLogFields(ctx, sc, draftVersionID, []library.LogFieldInput{
		{Label: "Chọn học sinh", Kind: library.LogFieldStudent},
	}); err != nil {
		return fmt.Errorf("seed: set log fields for %s: %w", templateDraftCode, err)
	}

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

// libraryMaterialSeed is one bank material seedLibraryBank ensures exists.
type libraryMaterialSeed struct {
	Title  string
	Kind   string
	URL    string
	Active bool
}

// libraryMaterialSeeds covers three of the v5 bank's new kinds plus one
// material left inactive (kind link, unrelated to the video/doc/note trio
// so the two groups stay easy to count independently in tests).
var libraryMaterialSeeds = []libraryMaterialSeed{
	{Title: "Video hướng dẫn phát âm", Kind: library.MaterialKindVideo, URL: "https://example.com/video/phat-am", Active: true},
	{Title: "Tài liệu ngữ pháp tổng hợp", Kind: library.MaterialKindDoc, URL: "https://example.com/doc/ngu-phap", Active: true},
	{Title: "Ghi chú chuẩn bị buổi học", Kind: library.MaterialKindNote, URL: "https://example.com/note/chuan-bi", Active: true},
	{Title: "Tài liệu cũ đã ngừng dùng", Kind: library.MaterialKindLink, URL: "https://example.com/link/ngung-dung", Active: false},
}

// libraryExerciseSeed is one bank exercise seedLibraryBank ensures exists.
// Code is left blank so the service assigns the next auto-generated
// "BT-0001"-style code.
type libraryExerciseSeed struct {
	Title string
	Skill string
	Level string
}

var libraryExerciseSeeds = []libraryExerciseSeed{
	{Title: "Bài tập đọc hiểu đoạn văn", Skill: "Đọc hiểu", Level: "A2"},
	{Title: "Bài tập viết đoạn văn ngắn", Skill: "Viết", Level: "B1"},
	{Title: "Bài tập từ vựng chủ đề gia đình", Skill: "Từ vựng", Level: "Cơ bản"},
}

// seedLibraryBank ensures the demo center-wide library bank exists: three
// materials of the new video/doc/note kinds plus one inactive material, and
// three exercises with skill/level and an auto-generated code. It returns
// the seeded exercises' ids, in the same order as libraryExerciseSeeds, for
// the draft template step to assign one into its exercise group.
func seedLibraryBank(
	ctx context.Context, db *gorm.DB, log *slog.Logger, svcs teachingMenuServices, sc authctx.Scope,
) ([]uuid.UUID, error) {
	for _, m := range libraryMaterialSeeds {
		if err := ensureLibraryMaterial(ctx, db, log, svcs, sc, m); err != nil {
			return nil, err
		}
	}
	exerciseIDs := make([]uuid.UUID, 0, len(libraryExerciseSeeds))
	for _, e := range libraryExerciseSeeds {
		id, err := ensureLibraryExercise(ctx, db, log, svcs, sc, e)
		if err != nil {
			return nil, err
		}
		exerciseIDs = append(exerciseIDs, id)
	}
	return exerciseIDs, nil
}

func ensureLibraryMaterial(
	ctx context.Context, db *gorm.DB, log *slog.Logger, svcs teachingMenuServices, sc authctx.Scope, spec libraryMaterialSeed,
) error {
	var ids []uuid.UUID
	if err := db.WithContext(ctx).Raw(
		"SELECT id FROM library_materials WHERE center_id = ? AND title = ? AND deleted_at IS NULL",
		sc.CenterID, spec.Title,
	).Scan(&ids).Error; err != nil {
		return fmt.Errorf("seed: look up material %q: %w", spec.Title, err)
	}
	if len(ids) > 0 {
		log.Info("seed: library material already exists, skipping", "title", spec.Title)
		return nil
	}

	url := spec.URL
	m, err := svcs.library.CreateMaterial(ctx, sc, library.MaterialRequest{Title: spec.Title, Kind: spec.Kind, URL: &url})
	if err != nil {
		return fmt.Errorf("seed: create material %q: %w", spec.Title, err)
	}
	if !spec.Active {
		if _, err := svcs.library.SetMaterialStatus(ctx, sc, m.ID, false); err != nil {
			return fmt.Errorf("seed: deactivate material %q: %w", spec.Title, err)
		}
	}
	log.Info("seed: library material created", "title", spec.Title, "kind", spec.Kind, "active", spec.Active)
	return nil
}

func ensureLibraryExercise(
	ctx context.Context, db *gorm.DB, log *slog.Logger, svcs teachingMenuServices, sc authctx.Scope, spec libraryExerciseSeed,
) (uuid.UUID, error) {
	var ids []uuid.UUID
	if err := db.WithContext(ctx).Raw(
		"SELECT id FROM library_exercises WHERE center_id = ? AND title = ? AND deleted_at IS NULL",
		sc.CenterID, spec.Title,
	).Scan(&ids).Error; err != nil {
		return uuid.Nil, fmt.Errorf("seed: look up exercise %q: %w", spec.Title, err)
	}
	if len(ids) > 0 {
		log.Info("seed: library exercise already exists, skipping", "title", spec.Title)
		return ids[0], nil
	}

	skill, level := spec.Skill, spec.Level
	e, err := svcs.library.CreateExercise(ctx, sc, library.ExerciseRequest{Title: spec.Title, Skill: &skill, Level: &level})
	if err != nil {
		return uuid.Nil, fmt.Errorf("seed: create exercise %q: %w", spec.Title, err)
	}
	log.Info("seed: library exercise created", "title", spec.Title, "code", e.Code)
	return e.ID, nil
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
