package centers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler exposes the center membership endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the centers handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// scope resolves the caller's center scope set by the ResolveScope
// middleware — the only sanctioned source of center identity; request bodies
// and paths never carry it.
func (h *Handler) scope(c *gin.Context) (authctx.Scope, bool) {
	s, ok := authctx.ScopeFrom(c)
	if !ok {
		response.Err(c, apperror.Unauthorized("authentication required"))
		return authctx.Scope{}, false
	}
	return s, true
}

// me returns the caller's center read model: the owner sees the full member
// roster (MeResponse), a member sees only the center's name (MemberMeResponse).
//
//	@Summary		Get my center
//	@Description	Owner gets the full member roster; a non-owner member gets only the center's name.
//	@Tags			centers
//	@Produce		json
//	@Success		200	{object}	response.Envelope{data=MeResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/centers/me [get]
func (h *Handler) me(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	resp, err := h.svc.Me(c.Request.Context(), scope)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// rename changes the center's name; owner only.
//
//	@Summary		Rename my center
//	@Tags			centers
//	@Accept			json
//	@Produce		json
//	@Param			request	body		RenameRequest	true	"new name"
//	@Success		200		{object}	response.Envelope{data=MeResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"not the owner"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/centers/me [patch]
func (h *Handler) rename(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var req RenameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	if err := h.svc.Rename(c.Request.Context(), scope, req); err != nil {
		response.Err(c, err)
		return
	}
	resp, err := h.svc.Me(c.Request.Context(), scope)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// removeMember offboards a member: owner-only. Closes the membership,
// disables the account, and revokes its refresh tokens — no new center is
// created.
//
//	@Summary		Remove a member (owner-only)
//	@Tags			centers
//	@Produce		json
//	@Param			teacherId	path	string	true	"teacher id"	format(uuid)
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}	"not the owner"
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}	"not a member of this center"
//	@Failure		422	{object}	response.Envelope{error=response.ErrorBody}	"owner cannot remove themselves"
//	@Security		BearerAuth
//	@Router			/centers/me/members/{teacherId} [delete]
func (h *Handler) removeMember(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	targetID, err := uuid.Parse(c.Param("teacherId"))
	if err != nil {
		response.Err(c, apperror.NotFound("member"))
		return
	}
	if err := h.svc.RemoveMember(c.Request.Context(), scope, targetID); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// permissions returns the permission-management read model; owner-only.
//
//	@Summary		Get the permission-management read model (owner-only)
//	@Description	Code-owned catalog (keys + Vietnamese labels), the center's roles with their permission sets, and non-owner members with role + overrides.
//	@Tags			centers
//	@Produce		json
//	@Success		200	{object}	response.Envelope{data=PermissionsResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}	"not the owner"
//	@Security		BearerAuth
//	@Router			/centers/me/permissions [get]
func (h *Handler) permissions(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	resp, err := h.svc.Permissions(c.Request.Context(), scope)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// replaceRolePermissions replaces a role's permission set; owner-only.
//
//	@Summary		Replace a role's permission set (owner-only)
//	@Description	PUT-replace semantics; keys are validated against the code-owned catalog.
//	@Tags			centers
//	@Accept			json
//	@Produce		json
//	@Param			roleId	path	string					true	"role id"	format(uuid)
//	@Param			request	body	RolePermissionsRequest	true	"full permission key list"
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}	"not the owner"
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}	"role not in this center"
//	@Failure		422	{object}	response.Envelope{error=response.ErrorBody}	"unknown or non-assignable key"
//	@Security		BearerAuth
//	@Router			/centers/me/roles/{roleId}/permissions [put]
func (h *Handler) replaceRolePermissions(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	roleID, err := uuid.Parse(c.Param("roleId"))
	if err != nil {
		response.Err(c, apperror.NotFound("role"))
		return
	}
	var req RolePermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	if err := h.svc.ReplaceRolePermissions(c.Request.Context(), scope, roleID, req); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// assignMemberRole assigns a member's role; owner-only.
//
//	@Summary		Assign a member's role (owner-only)
//	@Description	Role and member must both belong to the caller's center. The owner cannot be the target — they sit outside the role system.
//	@Tags			centers
//	@Accept			json
//	@Produce		json
//	@Param			teacherId	path	string				true	"teacher id"	format(uuid)
//	@Param			request		body	MemberRoleRequest	true	"role to assign"
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}	"not the owner"
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}	"role or member not in this center, or target is the owner"
//	@Failure		422	{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/centers/me/members/{teacherId}/role [put]
func (h *Handler) assignMemberRole(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	targetID, err := uuid.Parse(c.Param("teacherId"))
	if err != nil {
		response.Err(c, apperror.NotFound("member"))
		return
	}
	var req MemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	if err := h.svc.AssignMemberRole(c.Request.Context(), scope, targetID, req); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// replaceMemberOverrides replaces a member's override lists; owner-only.
//
//	@Summary		Replace a member's grant/deny overrides (owner-only)
//	@Description	PUT-replace semantics over both lists.
//	@Tags			centers
//	@Accept			json
//	@Produce		json
//	@Param			teacherId	path	string					true	"teacher id"	format(uuid)
//	@Param			request		body	MemberOverridesRequest	true	"full grant and deny key lists"
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}	"not the owner"
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}	"member not in this center, or target is the owner"
//	@Failure		422	{object}	response.Envelope{error=response.ErrorBody}	"unknown key, or key both granted and denied"
//	@Security		BearerAuth
//	@Router			/centers/me/members/{teacherId}/overrides [put]
func (h *Handler) replaceMemberOverrides(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	targetID, err := uuid.Parse(c.Param("teacherId"))
	if err != nil {
		response.Err(c, apperror.NotFound("member"))
		return
	}
	var req MemberOverridesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	if err := h.svc.ReplaceMemberOverrides(c.Request.Context(), scope, targetID, req); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// DashboardHandler exposes the owner dashboard endpoints.
type DashboardHandler struct {
	svc *Dashboard
}

// NewDashboardHandler builds the dashboard handler.
func NewDashboardHandler(svc *Dashboard) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

func (h *DashboardHandler) scope(c *gin.Context) (authctx.Scope, bool) {
	s, ok := authctx.ScopeFrom(c)
	if !ok {
		response.Err(c, apperror.Unauthorized("authentication required"))
		return authctx.Scope{}, false
	}
	return s, true
}

// pathUUID parses a path parameter; an unparsable id gets the dashboard's
// uniform denial rather than admitting the id shape was wrong.
func (h *DashboardHandler) pathUUID(c *gin.Context, name string) (uuid.UUID, bool) {
	v, err := uuid.Parse(c.Param(name))
	if err != nil {
		response.Err(c, apperror.Forbidden("not accessible from this center's dashboard"))
		return uuid.Nil, false
	}
	return v, true
}

// queryDate parses a required YYYY-MM-DD query parameter.
func (h *DashboardHandler) queryDate(c *gin.Context, name string) (time.Time, bool) {
	v, err := time.Parse("2006-01-02", c.Query(name))
	if err != nil {
		response.Err(c, apperror.Invalid("validation failed",
			map[string]string{name: "must be YYYY-MM-DD"}))
		return time.Time{}, false
	}
	return v, true
}

// dashboardClassSorts whitelists the public sort keys for the drill-down
// class list; the qualified columns match classes' own listing.
var dashboardClassSorts = map[string]string{
	"name":       "classes.name",
	"start_date": "classes.start_date",
	"created_at": "classes.created_at",
}

// teachers returns the roster with per-teacher activity counts; owner only.
//
//	@Summary		Dashboard: list teachers with activity counts
//	@Tags			centers
//	@Produce		json
//	@Success		200	{object}	response.Envelope{data=[]TeacherStatsResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}	"not the owner"
//	@Security		BearerAuth
//	@Router			/centers/dashboard/teachers [get]
func (h *DashboardHandler) teachers(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	resp, err := h.svc.Teachers(c.Request.Context(), scope)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// overview returns one month of per-class KPIs grouped by teacher; owner only.
//
//	@Summary		Dashboard: monthly per-class KPIs grouped by teacher
//	@Tags			centers
//	@Produce		json
//	@Param			month	query		string	false	"month as YYYY-MM, default current month"
//	@Success		200		{object}	response.Envelope{data=[]OverviewTeacherResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"not the owner"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"invalid month"
//	@Security		BearerAuth
//	@Router			/centers/dashboard/overview [get]
func (h *DashboardHandler) overview(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	resp, err := h.svc.Overview(c.Request.Context(), scope, c.Query("month"))
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// teacherClasses lists one teacher's classes for drill-down; owner only.
//
//	@Summary		Dashboard: list a teacher's classes
//	@Tags			centers
//	@Produce		json
//	@Param			teacherId	path		string	true	"teacher id"	format(uuid)
//	@Param			status		query		string	false	"active (default), archived, or all"
//	@Param			page		query		int		false	"page number, default 1"
//	@Param			per_page	query		int		false	"page size, default 20"
//	@Param			sort		query		string	false	"sort key: name, start_date, created_at"
//	@Success		200			{object}	response.Envelope{data=[]classes.ClassResponse,meta=response.Meta}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403			{object}	response.Envelope{error=response.ErrorBody}	"not the owner, or teacher not in this center"
//	@Failure		422			{object}	response.Envelope{error=response.ErrorBody}	"invalid status"
//	@Security		BearerAuth
//	@Router			/centers/dashboard/teachers/{teacherId}/classes [get]
func (h *DashboardHandler) teacherClasses(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	// Authorization outranks validation: a non-owner gets the same 403
	// whatever they send, learning nothing about parameter shapes. The
	// service re-checks ownership as the real gate.
	if err := requireDashboardView(scope); err != nil {
		response.Err(c, err)
		return
	}
	teacherID, ok := h.pathUUID(c, "teacherId")
	if !ok {
		return
	}
	filter := classes.ListFilter{}
	switch status := c.DefaultQuery("status", classes.StatusActive); status {
	case classes.StatusActive, classes.StatusArchived:
		filter.Status = status
	case "all":
		// Status stays empty: every non-deleted class.
	default:
		response.Err(c, apperror.Invalid("validation failed",
			map[string]string{"status": "must be one of: active, archived, all"}))
		return
	}
	params := pagination.Parse(c, "name", dashboardClassSorts)
	rows, total, err := h.svc.TeacherClasses(c.Request.Context(), scope, teacherID, filter, params)
	if err != nil {
		response.Err(c, err)
		return
	}
	out := make([]classes.ClassResponse, 0, len(rows))
	for i := range rows {
		out = append(out, classes.FromModel(&rows[i]))
	}
	response.List(c, out, params.Meta(total))
}

// classSessions lists a class's sessions in a date range with attendance
// stats; owner only.
//
//	@Summary		Dashboard: list a class's sessions with attendance stats
//	@Tags			centers
//	@Produce		json
//	@Param			teacherId	path		string	true	"teacher id"	format(uuid)
//	@Param			classId		path		string	true	"class id"		format(uuid)
//	@Param			from		query		string	true	"range start, YYYY-MM-DD"
//	@Param			to			query		string	true	"range end, YYYY-MM-DD"
//	@Success		200			{object}	response.Envelope{data=[]SessionRowResponse}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403			{object}	response.Envelope{error=response.ErrorBody}	"not the owner, or ids outside this center"
//	@Failure		422			{object}	response.Envelope{error=response.ErrorBody}	"invalid range"
//	@Security		BearerAuth
//	@Router			/centers/dashboard/teachers/{teacherId}/classes/{classId}/sessions [get]
func (h *DashboardHandler) classSessions(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	// Same authorization-before-validation order as teacherClasses.
	if err := requireDashboardView(scope); err != nil {
		response.Err(c, err)
		return
	}
	teacherID, ok := h.pathUUID(c, "teacherId")
	if !ok {
		return
	}
	classID, ok := h.pathUUID(c, "classId")
	if !ok {
		return
	}
	from, ok := h.queryDate(c, "from")
	if !ok {
		return
	}
	to, ok := h.queryDate(c, "to")
	if !ok {
		return
	}
	resp, err := h.svc.ClassSessions(c.Request.Context(), scope, teacherID, classID, from, to)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// session returns one session with its attendance sheet and revenue numbers;
// owner only.
//
//	@Summary		Dashboard: get one session with its attendance sheet
//	@Tags			centers
//	@Produce		json
//	@Param			sessionId	path		string	true	"session id"	format(uuid)
//	@Success		200			{object}	response.Envelope{data=SessionDetailResponse}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403			{object}	response.Envelope{error=response.ErrorBody}	"not the owner, or session outside this center"
//	@Security		BearerAuth
//	@Router			/centers/dashboard/sessions/{sessionId} [get]
func (h *DashboardHandler) session(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	sessionID, ok := h.pathUUID(c, "sessionId")
	if !ok {
		return
	}
	resp, err := h.svc.Session(c.Request.Context(), scope, sessionID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}
