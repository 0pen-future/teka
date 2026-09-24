# Kho Học Liệu (Library) — Current Implementation Inventory

**Date**: 2024-09-24  
**Scope**: Read-only codebase survey of complete feature implementation

---

## 1. Database Schema

### program_templates
| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() |
| center_id | UUID | NOT NULL, REFERENCES centers(id) ON DELETE CASCADE, UNIQUE(id, center_id) |
| code | VARCHAR(20) | NOT NULL, UNIQUE per center (live only, filtered by deleted_at) |
| name | VARCHAR(200) | NOT NULL |
| subject | VARCHAR(100) | nullable |
| level | VARCHAR(100) | nullable |
| description | TEXT | nullable |
| created_by | UUID | nullable, REFERENCES center_members(teacher_id, center_id) ON DELETE SET NULL |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT now() |
| updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT now() |
| deleted_at | TIMESTAMPTZ | nullable (soft delete) |

### program_template_versions
| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() |
| template_id | UUID | NOT NULL, REFERENCES program_templates(id, center_id) ON DELETE CASCADE |
| center_id | UUID | NOT NULL, UNIQUE(id, center_id) |
| version_no | INT | NOT NULL, CHECK (> 0), UNIQUE(template_id, version_no) |
| status | VARCHAR(12) | NOT NULL, CHECK IN ('draft', 'published', 'archived'), DEFAULT 'draft' |
| changelog | TEXT | nullable |
| published_at | TIMESTAMPTZ | nullable |
| score_set | JSONB | NOT NULL, DEFAULT '[]' (array of {key, label, max, weight}) |
| created_by | UUID | nullable, REFERENCES center_members(teacher_id, center_id) ON DELETE SET NULL |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT now() |
| updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT now() |
| **Unique Draft Index**: `(template_id) WHERE status = 'draft'` — at most one draft per template |

### template_lessons
| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() |
| version_id | UUID | NOT NULL, REFERENCES program_template_versions(id, center_id) ON DELETE CASCADE |
| center_id | UUID | NOT NULL, UNIQUE(id, center_id) |
| position | INT | NOT NULL, CHECK (> 0), UNIQUE DEFERRABLE (version_id, position) |
| title | VARCHAR(200) | NOT NULL |
| objectives | TEXT | nullable |
| duration_min | INT | nullable, CHECK (IS NULL or > 0) |
| homework_note | TEXT | nullable |
| prep_status | VARCHAR(10) | NOT NULL, DEFAULT 'todo', CHECK IN ('todo', 'doing', 'review', 'done') |
| assignee_id | UUID | nullable, REFERENCES center_members(teacher_id, center_id) ON DELETE SET NULL |
| due_date | DATE | nullable |
| checklist | JSONB | NOT NULL, DEFAULT '[]' (array of {label, done}) |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT now() |
| updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT now() |

### library_materials
| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() |
| center_id | UUID | NOT NULL, REFERENCES centers(id) ON DELETE CASCADE, UNIQUE(id, center_id) |
| title | VARCHAR(200) | NOT NULL |
| kind | VARCHAR(10) | NOT NULL, DEFAULT 'link', CHECK IN ('link', 'doc', 'video', 'other') |
| url | TEXT | nullable |
| description | TEXT | nullable |
| tags | JSONB | NOT NULL, DEFAULT '[]' (string array) |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT now() |
| updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT now() |
| deleted_at | TIMESTAMPTZ | nullable (soft delete) |

### library_exercises
| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() |
| center_id | UUID | NOT NULL, REFERENCES centers(id) ON DELETE CASCADE, UNIQUE(id, center_id) |
| title | VARCHAR(200) | NOT NULL |
| description | TEXT | nullable |
| difficulty | SMALLINT | nullable, CHECK (BETWEEN 1 AND 5) |
| tags | JSONB | NOT NULL, DEFAULT '[]' (string array) |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT now() |
| updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT now() |
| deleted_at | TIMESTAMPTZ | nullable (soft delete) |

### template_lesson_materials
| Column | Type | Constraints |
|--------|------|-------------|
| lesson_id | UUID | NOT NULL |
| material_id | UUID | NOT NULL |
| center_id | UUID | NOT NULL |
| shared_with_students | BOOLEAN | NOT NULL, DEFAULT FALSE |
| position | INT | NOT NULL, CHECK (> 0) |
| **PK**: (lesson_id, material_id) |
| FK: (lesson_id, center_id) → template_lessons ON DELETE CASCADE |
| FK: (material_id, center_id) → library_materials ON DELETE CASCADE |

### template_lesson_exercises
| Column | Type | Constraints |
|--------|------|-------------|
| lesson_id | UUID | NOT NULL |
| exercise_id | UUID | NOT NULL |
| center_id | UUID | NOT NULL |
| position | INT | NOT NULL, CHECK (> 0) |
| **PK**: (lesson_id, exercise_id) |
| FK: (lesson_id, center_id) → template_lessons ON DELETE CASCADE |
| FK: (exercise_id, center_id) → library_exercises ON DELETE CASCADE |

### template_log_fields
| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() |
| version_id | UUID | NOT NULL, REFERENCES program_template_versions(id, center_id) ON DELETE CASCADE |
| center_id | UUID | NOT NULL, UNIQUE(id, center_id) |
| position | INT | NOT NULL, CHECK (> 0), UNIQUE DEFERRABLE (version_id, position) |
| label | VARCHAR(100) | NOT NULL |
| kind | VARCHAR(10) | NOT NULL, CHECK IN ('text', 'number', 'select', 'checkbox') |
| options | JSONB | NOT NULL, DEFAULT '[]' (string array, required for kind='select') |
| required | BOOLEAN | NOT NULL, DEFAULT FALSE |

---

## 2. API Routes & Endpoints

All routes under `/library`, require `library.read` permission minimum. Publishing requires `library.publish`.

### Templates
| Method | Path | Permission | Request/Response | Business Rules |
|--------|------|-----------|------------------|-----------------|
| GET | `/library/templates` | library.read | Query: q, has_draft, page, per_page, sort; Response: TemplateResponse[] with prep summary | Returns live (non-soft-deleted) templates, published_version_no=highest published, draft_version_no=open draft, version_count=all versions |
| POST | `/library/templates` | library.edit | TemplateRequest {code, name, subject?, level?, description?, lesson_count?}; Response: TemplateResponse | Creates template + v1 draft, seeds N placeholder lessons if lesson_count given, 409 CODE_TAKEN if code exists (live) |
| GET | `/library/templates/:id` | library.read | Response: TemplateResponse | Single template with version summary |
| PUT | `/library/templates/:id` | library.edit | TemplateRequest; Response: TemplateResponse | Updates template fields only (not versions/lessons), 409 CODE_TAKEN on code collision |
| DELETE | `/library/templates/:id` | library.edit | Response: {deleted: true} | Soft-delete; versions/lessons retained, code becomes reusable |

### Versions
| Method | Path | Permission | Request/Response | Business Rules |
|--------|------|-----------|------------------|-----------------|
| GET | `/library/templates/:id/versions` | library.read | Response: VersionResponse[] | All versions, newest first, includes lesson_count |
| POST | `/library/templates/:id/versions` | library.edit | CreateVersionRequest {changelog?}; Response: VersionResponse | Opens new draft, copies lessons from latest published (or empty if no published), 409 DRAFT_EXISTS while draft open |
| POST | `/versions/:vid/publish` | library.publish | Response: VersionResponse | draft → published, locks all lessons, publishes_at set |
| POST | `/versions/:vid/archive` | library.edit | Response: VersionResponse | published → archived, lessons retained but version no longer "published" |
| GET | `/versions/:vid` | library.read | Response: VersionDetailResponse | Version + all lessons + materials/exercises + log_fields + score_set |

### Lessons
| Method | Path | Permission | Request/Response | Business Rules |
|--------|------|-----------|------------------|-----------------|
| GET | `/versions/:vid/lessons` | library.read | Response: LessonResponse[] | All lessons in order by position |
| POST | `/versions/:vid/lessons` | library.edit | LessonRequest; Response: LessonResponse | Appends new lesson, 409 VERSION_LOCKED if version not draft |
| PUT | `/lessons/:lid` | library.edit | LessonRequest; Response: LessonDetailResponse | Replaces title/objectives/duration_min/homework_note, 409 VERSION_LOCKED if version not draft |
| GET | `/lessons/:lid` | library.read | Response: LessonDetailResponse | Single lesson + materials + exercises |
| DELETE | `/lessons/:lid` | library.edit | Response: {deleted: true} | Removes lesson, renumbers rest, 409 VERSION_LOCKED if version not draft |
| PUT | `/versions/:vid/lessons/order` | library.edit | ReorderRequest {lesson_ids}; Response: LessonResponse[] | Reorder all lessons, must include every lesson exactly once, 409 VERSION_LOCKED if not draft |
| PATCH | `/lessons/:lid/prep` | library.edit | PrepRequest {prep_status?, checklist?}; Response: LessonResponse | Updates prep_status and/or replaces checklist (partial update), 409 VERSION_LOCKED if not draft |
| PATCH | `/lessons/:lid/assignment` | library.edit (needs prep.assign) | AssignmentRequest {assignee_id?, due_date?}; Response: LessonResponse | Replaces assignee and due_date as one block (null clears), 422 if assignee not live member, 409 VERSION_LOCKED |

### Preparation Board
| Method | Path | Permission | Request/Response | Business Rules |
|--------|------|-----------|------------------|-----------------|
| GET | `/versions/:vid/board` | library.read | Response: BoardResponse {template, version, columns[]} | Four fixed status columns (todo, doing, review, done) with lessons grouped by status, each card shows position, title, assignee_name, due_date, checklist_done/total |

### Materials & Exercises
| Method | Path | Permission | Request/Response | Business Rules |
|--------|------|-----------|------------------|-----------------|
| GET | `/library/materials` | library.read | Query: q, page, per_page, sort; Response: MaterialResponse[] | Paginated live materials, search by title |
| POST | `/library/materials` | library.edit | MaterialRequest; Response: MaterialResponse | Create material |
| GET | `/library/materials/:id` | library.read | Response: MaterialResponse | Single material |
| PUT | `/library/materials/:id` | library.edit | MaterialRequest; Response: MaterialResponse | Replace material fields, linked lessons see change at once |
| DELETE | `/library/materials/:id` | library.edit | Response: {deleted: true} | 409 MATERIAL_IN_USE if any lesson links it |
| GET | `/library/exercises` | library.read | Query: q, page, per_page, sort; Response: ExerciseResponse[] | Paginated live exercises, search by title |
| POST | `/library/exercises` | library.edit | ExerciseRequest; Response: ExerciseResponse | Create exercise |
| GET | `/library/exercises/:id` | library.read | Response: ExerciseResponse | Single exercise |
| PUT | `/library/exercises/:id` | library.edit | ExerciseRequest; Response: ExerciseResponse | Replace exercise fields, linked lessons see change at once |
| DELETE | `/library/exercises/:id` | library.edit | Response: {deleted: true} | 409 EXERCISE_IN_USE if any lesson links it |

### Lesson Attachments
| Method | Path | Permission | Request/Response | Business Rules |
|--------|------|-----------|------------------|-----------------|
| PUT | `/lessons/:lid/materials` | library.edit | LessonMaterialInput[] {material_id, shared_with_students}; Response: LessonMaterialResponse[] | Wholesale replace, body order = display order, 409 VERSION_LOCKED if not draft, 422 if material id not live |
| PUT | `/lessons/:lid/exercises` | library.edit | LessonExerciseInput[] {exercise_id}; Response: LessonExerciseResponse[] | Wholesale replace, body order = display order, 409 VERSION_LOCKED if not draft, 422 if exercise id not live |

### Log Fields & Score Set
| Method | Path | Permission | Request/Response | Business Rules |
|--------|------|-----------|------------------|-----------------|
| PUT | `/versions/:vid/log-fields` | library.edit | LogFieldInput[] {label, kind, options?, required}; Response: LogFieldResponse[] | Wholesale replace, body order = position, 409 VERSION_LOCKED if not draft, select kind must have ≥1 option (422 otherwise) |
| PUT | `/versions/:vid/score-set` | library.edit | ScoreComponentInput[] {key, label, max, weight}; Response: ScoreComponent[] | Wholesale replace, key unique within set (machine name), max > 0, 409 VERSION_LOCKED if not draft |

### Assignees
| Method | Path | Permission | Request/Response | Business Rules |
|--------|------|-----------|------------------|-----------------|
| GET | `/library/assignees` | library.edit or prep.assign | Response: AssigneeResponse[] {id, full_name} | Live center members eligible for lesson assignment (does NOT require members.list) |

**Key Business Rules**:
- **Publish Lock**: Published (status='published') and archived versions have locked lessons. Any write to a lesson or its attachments returns 409 VERSION_LOCKED unless version is draft.
- **Draft Uniqueness**: Each template has at most one draft version; attempt to create second draft returns 409 DRAFT_EXISTS.
- **Material/Exercise In-Use Check**: Delete operations check if material or exercise is linked to any lesson before allowing soft-delete; returns 409 MATERIAL_IN_USE or EXERCISE_IN_USE.
- **One-Way Transitions**: draft → published or draft → archived via explicit publish/archive endpoints; no direct status update endpoint.
- **Wholesale Replace**: Materials, exercises, log-fields, and score-set are all wholly replaced on save (no patch, no partial).

---

## 3. Web Pages & Routes

### `/library` — Main Library Page
**Permission**: library.read  
**Sections/Tabs**: 4 segmented tabs (Chương trình mẫu, Buổi học mẫu, Học liệu, Bài tập)

#### Tab: Chương trình mẫu (Templates)
- **Table Columns**: Chương trình (name, link), Mã (code), Môn · Trình độ (subject + level), Phát hành (published_version_no as badge), Bản nháp (draft_version_no as badge)
- **Actions** (if library.edit): Tạo chương trình mẫu button; per-row: click name → navigate to detail
- **Empty State**: "Chưa có chương trình mẫu nào" (no edit permission) or "Tạo chương trình đầu tiên bằng nút Tạo chương trình mẫu" (can edit)
- **Search**: Match on code or name, debounced

#### Tab: Buổi học mẫu (Lessons, read-only browser)
- **Controls**: HvSelect to pick template, shows selected version status
- **Table Columns**: STT (position), Tên buổi (title, link to lesson detail), Thời lượng (duration_min in minutes), Bài tập về nhà (yes/no)
- **Empty State**: "Chương trình này chưa có phiên bản nào" or "Phiên bản này chưa có buổi học nào"

#### Tab: Học liệu (Materials)
- **Table Columns**: Học liệu (title + description), Loại (kind badge), Đường dẫn (URL link, truncated), Thẻ (tags comma-separated)
- **Actions** (if library.edit): Thêm học liệu; per-row: Sửa, Xoá (refuses if in-use)
- **Search**: Match on title

#### Tab: Bài tập (Exercises)
- **Table Columns**: Bài tập (title + description), Độ khó (formatted as "Mức N"), Thẻ (tags)
- **Actions** (if library.edit): Thêm bài tập; per-row: Sửa, Xoá (refuses if in-use)
- **Search**: Match on title

---

### `/library/templates/new` — Create Template Wizard (new route, observed in code)
**Permission**: library.edit  
Creates template + first draft

---

### `/library/templates/:id` — Template Detail
**Permission**: library.read (edit actions require library.edit and/or library.publish)  
**Breadcrumb**: Kho học liệu (back link)  
**Header**: Template name, code, subject · level (if any), description (if any)  
**Version Picker**: HvSelect showing all versions (newest first), displays "v{N} · {status label}"; URL param ?v={version_no}

#### Section: Buổi học (Lessons)
- **Table**: Same columns as library-page Lessons tab
- **Actions** (if authoring = library.edit + draft):
  - Thêm buổi học button
  - Per-row: ↑ (move up), ↓ (move down), Xoá
  - Disabled on boundary rows
- **Lock Notice**: "Phiên bản này đã phát hành, nội dung được khoá. Tạo bản nháp mới để chỉnh sửa."
- **Controls**:
  - If draft + not authoring: Phát hành (publish) button
  - If published + authoring: Lưu trữ (archive) button
  - If no draft + authoring: Tạo bản nháp mới (opens dialog with changelog field)

#### Section: Nhật ký & Điểm (Grading)
- If authoring: LogFieldsEditor + ScoreSetEditor (row editors)
- Else: LogFieldsReadOnly + ScoreSetReadOnly (read-only displays)
- Each editor keyed on server state to reset on version switch or save

**Template Edit/Delete Buttons** (if library.edit):
- Sửa (opens TemplateDialog in edit mode)
- Xoá chương trình (confirmation dialog, 409 if in-use, redirects to /library on success)

---

### `/library/templates/:id/lessons/:lessonId` — Template Lesson Detail
**Permission**: library.read (edit requires library.edit + draft)  
**Breadcrumb**: {template.name} (back link to template)  
**Header**: "Buổi {position} · {title}", version status badge

#### If editable (library.edit + draft):
- **LessonEditor Form**: title, objectives, duration_min (text to number), homework_note → LessonFields form
- Save button, mutation pending

#### Else:
- **Lock Notice**: "Phiên bản v{N} đã phát hành, nội dung được khoá" or "Phiên bản v{N} đã lưu trữ, nội dung được khoá" or "Bạn không có quyền soạn buổi học mẫu"
- **ReadOnly Display**: 4 fields (Mục tiêu, Thời lượng, Bài tập về nhà) with "Chưa có" if null

#### LessonAttachments Section:
- **Materials Picker** (if editable):
  - Checklist over catalog, search
  - Checkbox per material, toggle "Chia sẻ với học viên" (shared_with_students)
  - "Đã chọn N học liệu" counter + Lưu học liệu button
  - Selected order = saved order (current first, then newly ticked in catalog order)
- **Materials ReadOnly** (else):
  - Per-material: position #, title (clickable URL if present), kind badge, "Chia sẻ HV" badge if shared_with_students
  - Empty: "Chưa gắn học liệu"

- **Exercises Picker** (if editable):
  - Checklist over catalog, search
  - Per-exercise: checkbox, title, difficulty label
  - "Đã chọn N bài tập" counter + Lưu bài tập button
- **Exercises ReadOnly** (else):
  - Per-exercise: position #, title, difficulty label
  - Empty: "Chưa gắn bài tập"

#### LessonPrepPanel (bottom):
- Prep status (4-column board: Cần làm, Đang làm, Chờ duyệt, Hoàn thành)
- Assignee picker (live members from /library/assignees)
- Due date (date picker, format YYYY-MM-DD)
- Checklist editor (array of {label, done})
- Only editable if draft + authoring

---

## 4. Web Components Inventory

### Pages
- **LibraryPage** (/library): Tabs host, lists templates/materials/exercises, template creation
- **TemplateDetailPage** (/library/templates/:id): Main authoring workspace
- **TemplateLessonPage** (/library/templates/:id/lessons/:lessonId): Lesson detail, content + prep + attachments
- **TemplateCreateWizardPage** (/library/templates/new): Creation flow (observed in routes)
- **PrepPage, PrepBoardPage, PrepAssignPage** (/prep routes): Preparation board and assignment UI (separate feature, not detailed here)

### Components
- **LessonFields**: Shared lesson form (title, objectives, duration_min, homework_note)
- **LessonDialog**: Modal to add lesson to draft
- **LessonsTable**: Ordered lessons of one version, edit/move/delete controls when authoring
- **LessonAttachments**: Materials + Exercises pickers/read-only displays
- **MaterialsTab**: Catalog search, add, edit, delete (CRUD) for materials
- **ExercisesTab**: Catalog search, add, edit, delete (CRUD) for exercises
- **MaterialDialog, ExerciseDialog**: Create/edit modals
- **TemplateDialog**: Create/edit template (code, name, subject, level, description)
- **TemplateFields**: Shared template form fields
- **LogFieldsEditor, LogFieldsReadOnly**: Row editor/display for log fields (text, number, select, checkbox)
- **ScoreSetEditor, ScoreSetReadOnly**: Row editor/display for score set (key, label, max, weight)
- **LessonPrepPanel**: Prep status + assignee + due_date + checklist (independent, keyed per lesson)

### Design System Components (Hv*)
Available in `/src/components/hv/`:
- **HvBadge** (hv-badge.tsx): `variant="success"|"warning"|"neutral"|"danger"|"info"`, `size="sm"|"md"`, `dot` prop
- **HvButton** (hv-button.tsx): `size="sm"|"md"|"lg"`, `variant="primary"|"secondary"|"ghost"|"danger"`, `disabled`, `pending`
- **HvSelect** (hv-select.tsx): Combobox with search, `options[]`, `value`, `onValueChange`, `placeholder`, `searchThreshold`, `sheetTitle`, `searchNoun`
- **HvSegmented** (hv-segmented.tsx): `variant="tabs"|"pills"`, `options[]`, `value`, `onValueChange`, `idBase` (for a11y)
- **HvStateBlock** (hv-state-block.tsx): `state="loading"|"error"|"empty"`, `title`, `description`, `action` (button)
- **HvModal** (hv-modal.tsx): `open`, `onOpenChange`, `title`, `description`, `footer`, children
- **HvConfirmDialog** (hv-confirm-dialog.tsx): `open`, `onOpenChange`, `title`, `description`, `confirmLabel`, `tone="danger"|"info"`, `pending`, `onConfirm`
- **HvNotice** (hv-notice.tsx): `tone="info"|"warning"|"danger"`, children (message)
- **HvIcon** (hv-icon.tsx): `name` (lucide icon names), `size`
- **HvChip** (hv-chip.tsx): Small tag/chip display
- **HvScoreInput** (hv-score-input.tsx): Number input with + / - buttons for score entry
- **ProgressBar, StatPill, StatusPill**: Specialized displays
- **HvToast**: Toast notification (hvToast function for imperative calls)
- All generate a11y-compliant HTML with ARIA labels

---

## 5. Material & Exercise Fields

### Material Kind Enum
Values: `"link"`, `"doc"`, `"video"`, `"other"`  
API validation: Required, oneof binding  
Web label mapping:
- link → "Liên kết"
- doc → "Tài liệu"
- video → "Video"
- other → "Khác"

### Material Fields (both API & Web)
- **id**: UUID
- **title**: string, 1–200 chars, required
- **kind**: one of above
- **url**: string nullable, optional, must be valid http(s) URL if given (max 2000 chars)
- **description**: string nullable, optional, max 4000 chars
- **tags**: string[], max 20 items, each max 50 chars
- **created_at, updated_at**: ISO timestamps

### Exercise Fields (both API & Web)
- **id**: UUID
- **title**: string, 1–200 chars, required
- **description**: string nullable, optional, max 4000 chars
- **difficulty**: nullable int, 1–5 (Mức 1–5), optional
- **tags**: string[], max 20 items, each max 50 chars
- **created_at, updated_at**: ISO timestamps

### Log Field Kind Enum
Values: `"text"`, `"number"`, `"select"`, `"checkbox"`  
Web label mapping:
- text → "Văn bản"
- number → "Số"
- select → "Chọn một"
- checkbox → "Đánh dấu"

### Log Field (Session Log Field)
- **id**: UUID
- **position**: int, 1-based, unique per version (wholesale replace)
- **label**: string, 1–100 chars, required
- **kind**: one of above
- **options**: string[], max 20, each max 100; required for kind='select', empty for others
- **required**: boolean

### Score Component (Score Set)
- **key**: string, machine identifier (a-z, 0-9, _, 1–30 chars), unique within version, required
- **label**: string, 1–100 chars, required
- **max**: float, > 0, required
- **weight**: float, >= 0, required (0 = not counted in totals)

---

## 6. Lesson & Template Features

### Lesson Fields
- **id**: UUID
- **version_id**: UUID (immutable)
- **position**: int, 1-based, contiguous, renumbered on delete
- **title**: string, 1–200 chars, required
- **objectives**: string nullable, max 4000 chars
- **duration_min**: int nullable, 1–1440 (minutes in a day), optional
- **homework_note**: string nullable, max 4000 chars
- **prep_status**: one of "todo" (Cần làm), "doing" (Đang làm), "review" (Chờ duyệt), "done" (Hoàn thành)
- **assignee_id**: UUID nullable (live center member)
- **due_date**: date (YYYY-MM-DD), nullable
- **checklist**: array of {label: string, done: bool}, max 50 items, default []
- **created_at, updated_at**: ISO timestamps

#### Prep fields (prep_status, assignee_id, due_date, checklist) belong to the draft they were set on
- Copying a version to a new draft resets them (not copied from previous version)
- Read-only on published/archived versions

### Template Fields
- **id**: UUID
- **code**: uppercase alphanumeric + hyphens, 2–20 chars, unique per center (live only)
- **name**: string, 1–200 chars
- **subject**: string nullable, 1–100 chars
- **level**: string nullable, 1–100 chars
- **description**: string nullable, max 2000 chars
- **created_by**: UUID (creator)
- **published_version_no**: int nullable (highest published version number)
- **draft_version_no**: int nullable (open draft, if any)
- **draft_version_id**: UUID nullable (for quick access)
- **version_count**: int (all versions, including archived)

#### Prep Summary (on draft)
Present only when template has an open draft:
- **lesson_count**: int (lessons in draft)
- **done_count**: int (lessons with prep_status = done)
- **assignees**: string[] (unique assignee names who have prep work assigned)

### Template Version Display
In web:
- Version selector shows all versions newest first
- Format: "v{version_no} · {status label}" (e.g., "v2 · Đã phát hành")
- Selection drives URL param ?v={version_no}, loads lessons of selected version
- Default: draft (if exists), else published, else newest by version_no

### Lesson Attachments
- Materials: ordered list of {material + shared_with_students flag}
- Exercises: ordered list of {exercise}
- Attachment UI: checklist picker (catalog + current), save replaces whole list
- Position tracked implicitly by order in body/response
- **Material "Chia sẻ với học viên" flag**: Boolean, controls visibility to students (not implemented in class view yet, stored in model)

### Template Versions → Lessons Copying
- New draft copies lessons + materials + exercises from latest published version
- Prep fields (status, assignee, due_date, checklist) NOT copied (reset to defaults: todo, null, null, [])
- Lesson content (title, objectives, duration_min, homework_note) IS copied
- Lesson attachments (materials/exercises) ARE copied

---

## 7. Web Zod Schemas (validation)

Located in `/src/features/library/schemas/library-schemas.ts`:
- `templateVersionStatusSchema`: enum draft|published|archived
- `prepStatusSchema`: enum todo|doing|review|done
- `materialKindSchema`: enum link|doc|video|other
- `logFieldKindSchema`: enum text|number|select|checkbox
- `programTemplateSchema`: matches TemplateResponse DTO
- `templateVersionSchema`: matches VersionResponse DTO
- `templateLessonSchema`: matches LessonResponse DTO
- `lessonMaterialSchema`: LessonMaterial (material + shared_with_students + position)
- `lessonExerciseSchema`: LessonExercise (exercise + position)
- `templateLessonDetailSchema`: Lesson + materials[] + exercises[]
- `materialSchema`, `exerciseSchema`, `logFieldSchema`, `scoreComponentSchema`: Standalone
- Form schemas: `templateFormSchema`, `lessonFormSchema`, `materialFormSchema`, `exerciseFormSchema`

---

## 8. Test Coverage

### Backend Tests (Go)
- **handler_test.go** (31.6 KB): HTTP handler tests for all endpoints
- **service_test.go** (48.5 KB): Business logic tests (publish lock, draft uniqueness, in-use checks, version copy, lesson renumbering)
- **integration_test.go** (36.7 KB): Full request→response cycles with database
- **items_test.go** (29.2 KB): Material and exercise CRUD

### Frontend Tests (Vitest + Playwright)
- **library-page.test.tsx**: Library main page (tabs, search, CRUD dialogs)
- **template-detail-page.test.tsx**: Template workspace (version picker, lesson table, editing)
- **template-lesson-page.test.tsx**: Lesson detail (form, prep panel, attachments)
- **template-create-wizard-page.test.tsx**: Creation flow (not detailed)
- **prep-page.test.tsx, prep-board-page.test.tsx, prep-assign-page.test.tsx**: Preparation board (not detailed)

### E2E Tests (Playwright)
- **library.spec.ts**: Complete user journey (create template, add lessons, reorder, publish, attach materials/exercises, cleanup)

---

## Summary of Current State

### Implemented Features
1. ✅ Program templates (CRUD, soft-delete, code uniqueness per center)
2. ✅ Template versioning (draft → published → archived, immutability, changelog)
3. ✅ Template lessons (ordered list, position management, content fields, soft prep fields)
4. ✅ Lesson preparation (4-column board, prep status, assignee + due_date, checklist)
5. ✅ Library materials (CRUD, soft-delete, kind enum, tags, shared_with_students flag on attachment)
6. ✅ Library exercises (CRUD, soft-delete, difficulty 1–5, tags)
7. ✅ Lesson attachments (wholesale replace, ordered, per-attachment flags)
8. ✅ Session log fields (text/number/select/checkbox, wholesale replace, client-side row editor)
9. ✅ Score components (key, label, max, weight, wholesale replace, client-side row editor)
10. ✅ Preparation board (4 fixed columns, card view with checklist progress)
11. ✅ Web UI (multi-tab library, template workspace, lesson detail, attachments, prep panel)
12. ✅ Design system components (Hv* library fully integrated)
13. ✅ Permissions (library.read, library.edit, library.publish, prep.assign)
14. ✅ Authorization (route-level + service-level checks)
15. ✅ Soft deletes (templates, materials, exercises; code/titles soft-deleted but searchable)

### Constraints & Behaviors
- **One draft per template** (unique index on status='draft')
- **Publish lock**: Published/archived lessons cannot be edited; draft-only mutations return 409 VERSION_LOCKED
- **In-use checks**: Materials/exercises refuse delete if any lesson links them (409 error)
- **Lesson position consistency**: Position is 1-based, contiguous, renumbered on delete
- **Wholesale replace**: Materials, exercises, log-fields, score-set are all wholly replaced, no patch
- **Prep fields not copied**: When creating new draft, lessons keep content but reset prep status/assignee/checklist
- **Soft delete reversal**: Template/material/exercise code/title becomes reusable after soft delete

---

**Status**: Complete inventory of current implementation  
**Last Updated**: 2024-09-24
