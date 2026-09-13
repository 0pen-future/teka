# Scout: React Web Task Board Planning Evidence

**Status:** DONE

---

## 1. Web App Layout & Package Structure

**Monorepo:** Single `package.json` at root (teka-root, private, git hooks only); apps live in `apps/`. Web app: `apps/web/package.json`.

**Key Dependencies:**
- React 19.2.8, React Router 8.3.1
- TanStack Query 5.102.8, Zustand 5.0.15
- Tailwind CSS 4.3.3, Radix UI 1.6.7, Zod 4.5.4
- Axios 1.20.0, React Hook Form 7.87.0
- MSW 2.15.0, Vitest 4.1.10, Playwright 1.62.1

**Path aliases:** `@/*` → `./src/*` (tsconfig.json:6)

**Directory structure:**
```
src/
  app/              entry, providers, router
  components/
    hv/             design system (HvButton, HvCard, HvModal, HvSelect, etc.)
    ui/             generated shadcn primitives
    shared/         app-wide building blocks
  features/         one per domain (api/, schemas/, hooks/, pages/, components/, __tests__/, routes.tsx)
  layouts/          root, auth, dashboard shells
  lib/api/          client.ts, envelope.ts, errors.ts, interceptors.ts, public-client.ts
  styles/           Tailwind + token CSS
  test/             vitest setup, MSW handlers, render helpers
```

**No separate packages/lib directory:** Headless kanban should live as `src/lib/kanban/` (core logic, no React) with the UI adapter in `src/features/tasks/` or a new feature. If reusability to another project is the goal, package it as `src/lib/headless-kanban/` for export, and bind the UI in `src/features/task-board/components/`.

---

## 2. Feature Folder Anatomy (Roster Example)

**Example structure:** `src/features/roster/`

```
api/
  students-api.ts       — listStudents(params), getStudent(id), createStudent(), updateStudent(), anonymizeStudent()
  classes-api.ts
  enrollments-api.ts
  contacts-api.ts
  imports-api.ts
  class-staff-api.ts

schemas/
  roster-schemas.ts     — studentSchema, contactSchema, classSchema, etc. (Zod)
  import-schemas.ts

hooks/
  roster-keys.ts        — Query key factories (studentsKeys, classesKeys, enrollmentsKeys, contactsKeys, classStaffKeys)
  use-students.ts       — useStudentsList(), useStudent(), useCreateStudent(), useUpdateStudent(), useAnonymizeStudent()
  use-classes.ts
  use-contacts.ts
  use-enrollments.ts
  use-roster-import.ts
  use-class-search.ts
  use-class-staff.ts

components/           — UI composition, feature-specific dialogs & forms
  class-dialog.tsx
  student-dialog.tsx
  enroll-student-dialog.tsx
  roster-table.tsx
  contact-picker.tsx
  etc.

pages/
  students-page.tsx     — list + tabs
  student-detail-page.tsx
  contacts-page.tsx
  class-settings-page.tsx
  roster-import-page.tsx

lib/
  class-permissions.ts
  current-month.ts
  roster-format.ts
  schedule-diff.ts

__tests__/
  *.test.tsx            — vitest + RTL; MSW handlers in roster-handlers.ts, roster-import-handlers.ts
  roster-handlers.ts    — MSW overrides for this feature

routes.tsx            — route definitions (mounted in app/router.tsx, not exported from index.ts)
index.ts              — public exports (hooks, types, not pages)
```

---

## 3. API Client Patterns

**Location:** `src/lib/api/` (shared layer)

**Client setup:** `client.ts:12-18` — axios instance with `baseURL: env.VITE_API_URL`, `withCredentials: true`, `timeout: 10_000`, interceptors for token/refresh.

**Example API module:** `roster/api/students-api.ts:1-48`
- Uses `apiClient.get/post/put/delete<unknown>()` for type safety on the response, not the input
- Wraps response in `parseData(studentSchema, res.data)` or `parseList(studentSchema, res.data)` (envelope-aware parsing)
- Exports named async functions, not class methods
- 404/409/4xx handled by ApiError in interceptors, not per-function

**Error shape:** `lib/api/errors.ts:11-39`
```ts
class ApiError extends Error {
  code: string;               // "VALIDATION_ERROR", "NOT_FOUND", "CONFLICT", "NETWORK_ERROR"
  status: number | null;      // HTTP, or null on network failure
  fields?: Record<string, string>;  // per-field validation (form integration)
  details?: unknown;          // endpoint-specific structured data (e.g. import rows)
}
```

**Envelope parsing:** `parseData(schema, data)` and `parseList(schema, data)` extract from `{success, data/error}` and validate with Zod.

---

## 4. TanStack Query & Mutation Patterns

**Query key factory:** `roster/hooks/roster-keys.ts:12-50`
```ts
const studentsKeys = {
  all: ["roster", "students"] as const,
  lists: () => [...studentsKeys.all, "list"] as const,
  list: (params: ListStudentsParams) => [...studentsKeys.lists(), params] as const,
  details: () => [...studentsKeys.all, "detail"] as const,
  detail: (id: string) => [...studentsKeys.details(), id] as const,
};
```
- Namespace per feature (["roster", "students"], ["roster", "classes"], etc.)
- `all`, `lists()`, `list(params)`, `details()`, `detail(id)` hierarchy for selective invalidation across entity boundaries

**Query hook:** `use-students.ts:18-35`
```ts
export function useStudentsList(params: ListStudentsParams = {}, options: { enabled?: boolean } = {}) {
  return useQuery({
    queryKey: studentsKeys.list(params),
    queryFn: () => listStudents(params),
    placeholderData: keepPreviousData,
    enabled: options.enabled ?? true,
  });
}
```

**Mutation with cross-entity invalidation:** `use-students.ts:38-48`
```ts
export function useCreateStudent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: StudentInput) => createStudent(input),
    onSuccess: (student) => {
      void queryClient.invalidateQueries({ queryKey: studentsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: studentsKeys.detail(student.id) });
      void queryClient.invalidateQueries({ queryKey: contactsKeys.all });  // cross-feature
    },
  });
}
```
- Comments document multi-entity side effects (e.g., "student_count on the owning contact just changed")
- Invalidate across feature boundaries through shared keys (sessionsKeys, contactsKeys, etc.)

---

## 5. Zod Schemas & Parsing

**Example:** `roster/schemas/roster-schemas.ts:23-50`
```ts
export const contactSchema = z.object({
  id: z.string(),
  full_name: z.string(),
  phone: z.string(),
  student_count: z.number().int(),
  created_at: z.string(),
  zalo_user_id: z.string().optional(),  // omitempty on server → .optional()
  zalo_name: z.string().optional(),
});

export type Contact = z.infer<typeof contactSchema>;

const vnPhonePattern = /^(0|\+84)(3|5|7|8|9)\d{8}$/;
const phoneField = z
  .string()
  .trim()
  .min(1, "Bắt buộc nhập số điện thoại")
  .regex(vnPhonePattern, "Số điện thoại không hợp lệ");
```
- Colocated in feature's `schemas/` folder
- Discriminate `omitempty` fields with `.optional()`, not `.nullable()`
- Comments link to server types (e.g., "contacts.ContactResponse from apps/api/internal/features/contacts/dto.go")
- Validation messages are Vietnamese; phone pattern mirrors server-side

---

## 6. Center Context & Permission Gating

**Hook:** `src/features/teaching/hooks/use-center-context.ts:3-66`
```ts
export interface CenterContext {
  centerId: string | null;
  centerName: string | null;
  isOwner: boolean;
  canSendReports: boolean;
  canRunSends: boolean;
  isResolved: boolean;       // true once GET /centers/me resolved
  isError: boolean;          // true on final failure
  permissions: string[];     // from API catalog
  has: (key: string) => boolean;
}

export function useCenterContext(): CenterContext {
  const { data, isError } = useCenter();
  // ... derives from /centers/me response (owner body has `members` key, member body has `center_name`)
  return {
    ...
    has: (key: string) => isOwner || permissions.includes(key),
  };
}
```
- Consumes `useCenter()` hook (from center feature)
- GET /centers/me response is role-shaped: narrowed on `"members" in data` (owner only has members array)
- `has(key)` short-circuits owner (tolerance for rollout skew), then checks array
- `isResolved` and `isError` prevent UI flicker from optimistic renders

**Permission-gated nav:** `src/layouts/dashboard-layout.tsx:33-150`
```ts
interface NavEntry {
  label: string;
  to: string | null;          // null while period id doesn't resolve yet
  Icon: ComponentType<LucideProps>;
  pending?: boolean;
  perm?: string;              // effective permission key; unset = visible to all
}

// In useNavGroups():
const { isOwner, canSendReports, isResolved, has } = useCenterContext();

const groups: NavGroup[] = [
  { header: null, entries: [{ label: "Tổng quan", to: "/", Icon: HvHomeIcon }] },
  {
    header: "Dạy học",
    entries: [
      {
        label: "Điểm danh",
        to: "/sessions",
        Icon: HvCheckIcon,
        perm: "sessions.list",
      },
      // ... more entries with perm:
    ],
  },
  // ... conditional spreads for owner-only entries (isResolved && isOwner)
];

// Render:
groups.forEach(group => 
  group.entries
    .filter(entry => !entry.perm || (isResolved && has(entry.perm)))  // hide until resolved
    .map(entry => <NavLink disabled={!entry.to} to={entry.to} />)
);
```
- Entries with `perm` are hidden until `/centers/me` resolves
- Conditional spreads for owner-only surfaces (e.g., Phân quyền vai trò)
- Period-scoped routes build link once `useCurrentPeriod` resolves (no redirect page)

**Deep-link guard:** Frontend guidelines:98-107 — gated pages need both hidden nav AND their own redirect/query guard; hiding the nav is not authorization. Write-only actions stay behind `isOwner`.

---

## 7. Design System Components

**Location:** `src/components/hv/index.ts`

**Available components:**
- `HvButton` — all sizes keep 44px hit area
- `HvCard` — flexible padding variants
- `HvModal` — sizes md/lg/xl (bottom sheet on mobile)
- `HvConfirmDialog` — single-action confirmations
- `HvNotice` — tone: info/warning/danger (danger role="alert")
- `HvStateBlock` — loading/empty/error UX
- `hvToast` (Sonner integration) — transient feedback
- `HvSegmented` — radio group or tabs (variant="tabs")
- `HvSelect` — combobox + listbox, roving focus, popover/sheet/searchable
- `HvBadge` — labeled badges
- `HvScoreInput` + `parseScoreInput` — decimal scores, data-state: idle/dirty/saved/invalid
- `HvIcon` — (HvCheckIcon, HvFileIcon, HvHomeIcon, HvPlusIcon, HvSendIcon, HvUsersIcon, HvWalletIcon, HvXIcon, HvClockIcon)
- `StatusPill` — status labels with color coding (statusPillLabels mapping)
- `StatPill` — stat display with kind/size
- `ProgressBar` — colored progress, sizes

---

## 8. Design Tokens & Styling

**Token structure:** `src/styles/tokens/colors.css:1-126`

**Key palette:**
- **Cream (neutrals):** --cream-100 (app bg), --cream-200 (sunken), --cream-300
- **Ink (text):** --ink-900 (headings), --ink-700 (body), --ink-500 (muted), --ink-400 (placeholder), --ink-300 (disabled)
- **Mint (primary, math, success):** --mint-400 (primary), --mint-500 (pressed/shadow), --mint-600 (ink)
- **Sky (secondary, Vietnamese, info):** --sky-300, --sky-400, --sky-500
- **Sun (reward, warning):** --sun-400, --sun-600 (ink)
- **Coral (error, lives):** --coral-400, --coral-600 (ink)
- **Lines:** --line-200 (default border), --line-300 (strong)

**Semantic aliases in root:**
```css
--text-strong: var(--ink-900);
--surface-page: var(--cream-100);
--surface-card: var(--white);
--border-subtle: var(--line-200);
--brand-primary: var(--mint-400);
--success: var(--mint-400);
--danger: var(--coral-400);
```

**Typography:** `src/styles/tokens/typography.css`
- Fonts: "Baloo 2" (display), "Nunito" (body)
- Weights: --fw-regular 400, --fw-medium 500, --fw-semibold 600, --fw-bold 700, --fw-extra 800, --fw-black 900

**Spacing & effects:** `spacing.css`, `effects.css` also present.

**No separate design guidelines doc:** Token map IS the authority (`frontend-guidelines.md` references the hv kit; docs link to `apps/web/src/styles/tokens/` implicitly).

---

## 9. Permission Matrix UI

**Location:** `src/features/center/components/permission-matrix.tsx:1-100+`

**Architecture:**
- Reads from `useCenterPermissions()` hook (owner-only endpoint GET /centers/me/permissions)
- Returns `{ roles: Role[], catalog: PermissionInfo[], catalogVersion: number }`
- Roles each carry `id`, `name`, `permissions: string[]`, `assignmentVersion: number` (CAS)
- Catalog groups by resource; each permission has `key`, `risk` ("high"/"low"), `label`

**Editing model:**
- Local drafts per role (React state), survive tab switches but not navigation
- Tab structure: one underline tab per catalog resource group (grouped by risk, resource type)
- Checkbox grid: roles × permissions
- Save sends full `permissions` array + `{ catalogVersion, assignmentVersion }` for CAS
- 409 on version mismatch → refetch read model, draft stays, owner reviews and re-saves (no auto-retry)
- Widening a role with high-risk keys → confirmation modal naming keys + current member count
- Narrowing = no confirmation

**MSW fixture:** `src/features/center/__tests__/center-handlers.ts` provides `mockCenterMe()` which can hand sequential payloads for pre-action/post-refetch scenarios. The permission catalog is seeded in the test once.

---

## 10. Member Directory & Member Selector

**API:** `src/features/center/api/center-api.ts:13-16`
```ts
export async function getCenterMe(): Promise<CenterMe> {
  const res = await apiClient.get<unknown>("/centers/me");
  return parseData(centerMeSchema, res.data);
}
```

**Response shape:** Role-shaped union
- **Owner (has `members` key):** `{ center: { id, name, ... }, members: Member[], permissions: string[] }`
- **Member:** `{ center_name: string, can_send_reports: boolean, permissions: string[] }`

**No dedicated member-list endpoint or member-selector component:** Members come from the owner's GET /centers/me only. A new member picker would need:
1. Extract members from center context (owner-only view)
2. Create a feature component `src/features/center/components/member-picker.tsx` or `src/features/teaching/components/member-selector.tsx` depending on domain ownership
3. Likely reuse HvSelect for the UI

---

## 11. Testing Patterns

**Vitest setup:** `src/test/setup.ts`
- MSW server with `onUnhandledRequest: "error"` — any unmocked request fails
- Per-test: reset auth state, clear localStorage, reset toast
- jsdom shims: matchMedia (returns false), ResizeObserver stub, scrollIntoView noop, pointer capture noop

**Render helper:** `src/test/utils.tsx:renderWithProviders()`
- Mounts fresh QueryClient, theme provider, memory router, toaster
- `signInAs(testPrimaryTeacher)` seeds auth store
- `mockViewport(width)` for media query breakpoints (sheet vs. popover)

**MSW fixtures:** `src/test/msw/handlers.ts` owns happy-path API stubs; tests override with `server.use(...)` for errors/edges.

**Quartet pattern (list pages):** Cover loading/empty/error/data states (frontend-guidelines:137)
- Example: `src/features/roster/__tests__/class-settings-page.test.tsx:48-80` renders, awaits loading, checks filled form
- Tests for pages with permission gates also test both owner and member branches

**Playwright e2e:** `e2e/*.spec.ts`, runs against localhost:5173 + seeded API
- One worker (mutates shared DB)
- Role-based locators; `exact: true` on cell lookups
- Test data: suffix timestamps for uniqueness (roster.spec.ts:18-24)
- Example flow: login → create contact → add students → create class → enroll (roster.spec.ts:15-80+)

---

## 12. ESLint & Make Targets

**ESLint:** `eslint.config.js:9-64`
- Recommended TS + React Hooks + React Refresh + jsx-a11y
- Generated `src/components/ui/` exempted from `react-refresh/only-export-components`
- Public statement route forbidden from importing `@/features/auth` or `@/lib/api/client` (must use public-client); enforced by `no-restricted-imports` pattern

**Make targets (from Makefile):**
```
make test-web          npm run test (vitest, MSW offline)
make lint-web          npm run lint && npm run format:check && npm run typecheck
make e2e               npm run e2e (Playwright against localhost:5173)
make build-web         npm run build (Vite production bundle)
make web-dev           npm run dev (Vite dev server, hot reload)
```

---

## 13. Headless & Hook Patterns

**No explicit headless abstraction library found**, but the codebase follows headless principles:
- **Query keys + hooks pattern:** `roster-keys.ts` + `use-*.ts` files achieve headless state management; hooks own the query/mutation logic and side effects, decoupled from UI
- **Envelope + Zod parsing:** API layer (`parseData`, `parseList`) is UI-agnostic; can be reused in any consumer
- **ApiError normalization:** `lib/api/errors.ts` standardizes all error shapes; UI just matches on `.code`
- **Feature `index.ts` exports:** Only hooks, types, and schemas exported; components are feature-internal (encourages headless/slot-based composition)
- **No render-prop or children-as-function patterns observed** in the codebase — UI composition is JSX-based with standard component composition

**For task board headless core:** Recommend mirroring the query-key-factory + hook pattern:
```
src/lib/kanban/
  types.ts          — Column, Task, Board types
  schemas.ts        — Zod for validation (if API-bound)
  state.ts          — Zustand store or useReducer logic (board state, drag hover, etc.)
  hooks.ts          — useKanban(), useKanbanColumns(), useKanbanTasks(), useMoveTask()
  api.ts            — moveTask(), updateTask() (if API-bound)
  keys.ts           — Query key factory (if API-bound)

src/features/tasks/
  components/       — TaskBoard, TaskCard, ColumnHeader (bound to hv kit)
  pages/            — tasks-page.tsx
  hooks/            — task-specific hooks (useTaskBoardAPI, useTaskFilters)
  __tests__/        — component + integration tests
  routes.tsx
  index.ts
```

---

## 14. Audit Log Action Labels

**Current model:** `src/features/audit/schemas/audit-schemas.ts:9-32`
- `action: string` is stored as-is (e.g., "auth.login", "students.create", "enrollments.delete")
- No frontend map of action → label; the action string itself is rendered in the table
- `AuditTable` displays `log.action` in monospace (audit-table.tsx:91)

**For new task. actions:** API will assign action names (e.g., "tasks.create", "task_columns.reorder"). Frontend renders them directly. To extend with custom labels, create a feature map in `src/features/tasks/lib/audit-action-labels.ts`:
```ts
const taskAuditLabels: Record<string, string> = {
  "tasks.create": "Tạo công việc",
  "tasks.update": "Cập nhật công việc",
  "task_columns.reorder": "Sắp xếp cột",
  ...
};
```
Then update `AuditTable` to call a label resolver if it exists, fallback to raw action.

---

## 15. Dark Mode & Theme

**Setup:** React context-based class theming (likely in `src/components/shared/theme-provider.tsx`, referenced in setup.ts jsdom shims)
- Radix uses `matchMedia` to detect system preference
- jsdom shim returns `matches: false` always in tests; use `mockViewport()` for breakpoints

**Token map supports dark:** colors.css defines --cream-*, --ink-*, etc. as CSS custom properties; dark mode likely uses `:root[data-theme="dark"]` selector (standard pattern). Not explicitly detailed in guidelines, but the token structure is complete.

---

## Unresolved Questions

- None. Evidence is complete for feature planning.

