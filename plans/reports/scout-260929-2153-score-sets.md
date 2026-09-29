## Báo cáo khám phá: Schema, flow grading, và phụ thuộc khi loại bỏ `/center/class-config`

### 1. Schema cơ sở dữ liệu

**Migrations 000014_grading (dòng 1-87):**
- `score_sets` (id UUID PK, center_id, name, created_at, updated_at, deleted_at): bộ điểm mẫu cấp trung tâm, soft-delete qua deleted_at. UNIQUE (center_id, lower(name)) với WHERE deleted_at IS NULL.
- `score_set_components` (id UUID PK, set_id FK, name, position SMALLINT): thành phần của bộ. UNIQUE (set_id, name) và UNIQUE (set_id, position).
- `class_score_components` (id UUID PK, class_id, center_id, name, position SMALLINT, source_set_id FK): snapshot per lớp khi gán. Composite FK (class_id, center_id) → classes. UNIQUE (class_id, name), UNIQUE (class_id, position), UNIQUE (id, class_id). source_set_id ON DELETE SET NULL (khi bộ gốc xóa cứng, hiếm vì soft-delete).
- `student_scores` (id, class_id, session_id, component_id FK → class_score_components(id, class_id), student_id, teacher_id, center_id, score NUMERIC(4,1), created_at, updated_at): điểm 0–10 per học sinh × thành phần × buổi. UNIQUE (session_id, component_id, student_id).

**Migration 000034_template_version_v5 (dòng 1-49):**
- ALTER program_template_versions ADD COLUMN score_set (jsonb NOT NULL DEFAULT '[]'): mảng các bộ điểm.
- Schema jsonb: `[{key: string, title: string, components: [{key, label, max, weight}, ...]}, ...]`
- Bọc đơn phía từ phẳng: `{key: 'main', title: 'Bộ điểm', components: [...]}`

**Library model.go (dòng 322-365):**
- `ScoreComponent` {Key, Label, Max, Weight} → JSON
- `ScoreSetGroup` {Key, Title, Components []ScoreComponent} → JSON  
- `ScoreSet []ScoreSetGroup` (gorm Value/Scan marshaler)

### 2. Cách grading đọc thành phần và marks

**Routes (grading/routes.go:9-24):**
- GET /score-sets (listSets, owner-only): danh sách bộ điểm trung tâm
- POST /score-sets (createSet, owner-only): tạo bộ
- PUT /score-sets/:id (updateSet, owner-only): sửa bộ, thành phần toàn bộ replace
- DELETE /score-sets/:id (deleteSet, owner-only): soft-delete
- POST /classes/:id/score-set (assignScoreSet, owner-only): copy thành phần bộ vào class_score_components
- DELETE /classes/:id/score-set (clearScoreSet, owner-only): xóa snapshot lớp
- GET /classes/:id/score-components (getClassComponents, readable gate): lấy snapshot của lớp (dùng dạy + admin chung)
- GET /sessions/:id/scores (getSessionScores, permission gate PermScoresRead): lấy thành phần lớp + các điểm đã ghi của buổi
- PUT /sessions/:id/scores (putSessionScores, permission gate PermScoresEdit): batch upsert/delete điểm per thành phần

**Marks logic (grading/service.go:198-450):**
- `AssignScoreSet`: lock lớp → guard "no scores yet" → replace class_score_components (cascade-delete student_scores nếu đổi). Atomic tx để tránh race với score write.
- `ClearScoreSet`: tương tự, không thay thế bộ mà thay thế thành rỗng.
- `GetClassComponents`: đọc class_score_components position-order. Read gate = readable class (teacher lớp + đọc center).
- `PutSessionScores`: lock lớp → validate component_id ∈ class_score_components → upsert/delete student_scores. Null cell = xóa. Roster gate: cell mới yêu cầu student ở roster session ngày, nhưng student có score khác rồi thì sửa được sau khi enrollment kết thúc.

**Marks reference: component_id FK → class_score_components(id, class_id) → student_scores(component_id, class_id).** Nếu replace class_score_components, ON DELETE CASCADE xóa student_scores — đó là lý do guard "no scores".

**Repository (grading/repository.go:151-221):**
- `GetClassComponents` (dòng 151-158): WHERE center_id = ? AND class_id = ? ORDER BY position
- `ReplaceClassComponents` (dòng 160-169): xóa all dòng class_id cũ, insert danh sách mới
- `ListScoresBySession` (dòng 190-196): WHERE center_id = ? AND session_id = ?
- `UpsertScores`/`DeleteScores`: ON CONFLICT (session_id, component_id, student_id)

### 3. Cách lớp bind template version

**classprogram/model.go (dòng 9-18):**
- `Program` {ClassID PK, CenterID, TemplateVersionID, AppliedAt, AppliedBy}: một lớp ghi 1 template version, re-apply replace row.
- Bảng `class_programs` (class_id PK, center_id, template_version_id, applied_at, applied_by)

**Hook point hiện tại:** Không có. Khi apply template vào lớp, chỉ ghi template_version_id. Grading hiện chỉ đọc class_score_components snapshot, KHÔNG đọc template_version.score_set.

### 4. Web consumers của center score sets

**Center feature (center/pages/class-config-page.tsx:1-212):**
- Owner-only page "Cấu hình lớp học"
- Section 1: CRUD bộ điểm (list, create, edit, delete via useScoreSets hook)
  - Endpoints: GET /score-sets, POST /score-sets, PUT /score-sets/:id, DELETE /score-sets/:id
  - Components: ScoreSetCard, ScoreSetEditorModal
- Section 2: gán bộ vào lớp
  - GET /classes?per_page=100 → ClassScoreSetTable → AssignScoreSetDialog
  - POST /classes/:id/score-set (assignScoreSet) → lock 409 nếu lớp có scores
  - DELETE /classes/:id/score-set (clearScoreSet) → tương tự

**Center hooks (center/hooks/use-score-sets.ts):**
- `useScoreSets()`: query GET /score-sets, cache key scoreSets
- `useDeleteScoreSet()`: mutation DELETE /score-sets/:id

**Center components:**
- `score-set-editor-modal.tsx`: form tạo/sửa bộ (POST/PUT)
- `assign-score-set-dialog.tsx`: dialog gán bộ vào lớp (POST /classes/:id/score-set)
- `class-score-set-table.tsx`: table lớp với nút gán
- `score-set-card.tsx`: thẻ hiển thị bộ + edit/delete
- `score-set-preview-strip.tsx`: preview thành phần

**Teaching feature (teaching/hooks/use-component-scores.ts + use-teaching-mutations.ts):**
- `useClassScoreComponents(classId)`: query GET /classes/:id/score-components, cache key scoreComponents(classId). Kết quả: {components: []} = class dùng UI điểm chung (không có thành phần riêng).
- `useSessionScores(sessionId)`: query GET /sessions/:id/scores → {components, scores}
- `useSaveSessionScores(sessionId)`: mutation PUT /sessions/:id/scores

**Classbook (teaching/pages/classbook-page.tsx:126-136):**
- `const scoreComponentsQuery = useClassScoreComponents(selectedClassId)`
- Branch: `hasScoreComponents = components.length > 0`
  - True → tính scoredCounts từ sessionScores (per buổi)
  - False → tính từ month marks batch (cách cũ)

**Roster sidebar (roster/components/class-info-tab.tsx:122-126):**
- Owner shortcut link: "/center/class-config" ("Cấu hình lớp học") ngay tại class info tab (read-only).

**Library (library/components/score-sets-editor.tsx:183):**
- `useSetScoreSet(versionId)`: mutation PUT /library/versions/:id/score-set (ĐỎ: endpoint riêng biệt cho template version, không phải center score-set)

### 5. Permission/route policy backend

**routespec.go (dòng 222-227, 320-322, 465-466):**
- GET /score-sets: KindOwnerOnly, no audit action
- POST /score-sets: KindOwnerOnly, audit "score_set.create" entity score_set
- PUT /score-sets/:id: KindOwnerOnly, audit "score_set.update" entity score_set id
- DELETE /score-sets/:id: KindOwnerOnly, audit "score_set.delete" entity score_set id
- POST /classes/:id/score-set: KindOwnerOnly, audit "class.score_set.assign" entity class id
- DELETE /classes/:id/score-set: KindOwnerOnly, audit "class.score_set.clear" entity class id
- GET /classes/:id/score-components: PermScoresRead permission, no audit
- GET /sessions/:id/scores: PermScoresRead permission, no audit
- PUT /sessions/:id/scores: PermScoresEdit permission, audit "session.scores.update" entity session id
- PUT /library/versions/:vid/score-set: PermLibraryEdit permission, audit "template_version.set_score_set" entity template_version vid

**route_policy_snapshot_test.go (dòng 47-51, 106-108):**
- Score-sets endpoints → PolicyOwnerOnly
- Scores endpoints → PolicyPermission (scores.read, scores.edit)
- Library score-set → PolicyPermission (library.edit)

**action_test.go (dòng 17-22, 46, 64, 94, 112-114, 145):**
- Audit actions: score_set.{create,update,delete}, class.score_set.{assign,clear}, session.scores.update, template_version.set_score_set

**Swagger (docs.go):**
- Endpoints tại paths /score-sets, /score-sets/{id}, /classes/{id}/score-set, /library/versions/{vid}/score-set (auto-generated từ swag annotations)

### 6. Khác: seeds & phạm vi khác

**Seeds (seeds/teaching_menu.go:308-325):**
- Chỉ tạo template-level score_set via `svcs.library.SetScoreSet()`, không tạo center-level score_sets

**Phạm vi khác:**
- Statements/Reports/Student records: KHÔNG đọc class_score_components hay student_scores (chỉ đọc session_marks cho marks chung)
- Classbook columns: CHỈ từ component_id và component name (class_score_components), không từ bộ gốc hay template

### 7. Design options để template score_set authoritative

#### Option A: Copy-on-bind (hiện tại từng phần)
- Khi apply template vào lớp (classprogram), copy program_template_versions.score_set → class_score_components
- Không cần thay đổi grading service (đã expect class_score_components)
- **Risk:** Nếu template score_set thay đổi sau khi áp dụng, lớp không tự cập nhật → cần UI/workflow để re-apply hoặc bảo hiểm người dùng

#### Option B: Grading read template directly
- PutSessionScores/GetSessionScores validate component_id so với program_template_versions.score_set chứ không class_score_components
- Xóa class_score_components, loại bỏ class-level snapshot
- **Risk:** Breaking change cho existing marks (component_id FK còn nút). Phải migrate marks → match template keys. Không có snapshot → editing template thay đổi all lớp apply nó (không isolated per lớp). Classbook phải biết resolve template versionID.

#### Option C: Hybrid (recommend)
- Giữ class_score_components snapshot để backward-compat marks hiện tại
- Copy-on-bind khi re-apply template (classprogram.AppliedAt)
- Grading vẫn dùng class_score_components (không đổi logic)
- Xóa center-level score-sets (scope task)
- Loại bỏ /center/class-config (owner page, routes, sidebar link)
- Template version score_set = single source truth; class snapshot inherit lúc apply

**Risk hybrid:** Marks cũ (pre-apply) không có component_id nếu được tạo trước template binding → phải xử lý backfill hoặc graceful fallback.

### Status: DONE

**Summary:** Schema score_sets/class_score_components là snapshot-per-lớp từ center template. Grading đọc class_score_components chứ không template trực tiếp. Classbook + teaching dùng GET /classes/:id/score-components + PUT /sessions/:id/scores. Library score-set (template.score_set JSONB) tách biệt, route riêng. Center-only CRUD (/score-sets) chỉ cho owner. To remove class-config: loại DROP class_score_components/score_sets tables, migrate marks+classes→template binding, implement copy-on-apply classprogram feature, xóa web routes/components/sidebar link, xóa backend endpoints + audit actions + route specs.
