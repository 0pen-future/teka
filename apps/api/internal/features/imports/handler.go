package imports

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
)

// maxUploadBytes caps the workbook a single request may carry. It bounds the
// network path; the parser applies its own unzip and row limits, because a
// compressed workbook well under this cap can still decompress to far more.
const maxUploadBytes = 2 << 20 // 2 MiB

// xlsxContentType is the OOXML spreadsheet media type browsers expect for a
// download; anything else and Excel refuses to open the file.
const xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// Handler binds HTTP to the imports service.
type Handler struct {
	svc *Service
}

// NewHandler builds the imports handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// scope resolves the authenticated caller's center scope — the only sanctioned
// source of tenant identity. The uploaded workbook never names a center.
func (h *Handler) scope(c *gin.Context) (authctx.Scope, bool) {
	sc, ok := authctx.ScopeFrom(c)
	if !ok {
		response.Err(c, apperror.Unauthorized("authentication required"))
		return authctx.Scope{}, false
	}
	return sc, true
}

// template streams the blank workbook.
//
// @Summary      Tải file Excel mẫu để import lớp và học sinh
// @Description  Trả về file .xlsx gồm 2 sheet (Lop, HocSinh) với dòng tiêu đề và một dòng ví dụ.
// @Tags         imports
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Success      200  {file}    binary
// @Failure      401  {object}  response.Envelope
// @Failure      403  {object}  response.Envelope
// @Security     BearerAuth
// @Router       /imports/roster/template [get]
func (h *Handler) template(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	b, err := h.svc.Template(c.Request.Context(), sc)
	if err != nil {
		response.Err(c, err)
		return
	}
	// Deliberately outside the response envelope, like the health probes: this
	// is a binary stream, not JSON.
	c.Header("Content-Disposition", `attachment; filename="teka-import-mau.xlsx"`)
	c.Data(http.StatusOK, xlsxContentType, b)
}

// importRoster runs a workbook, either as a check or for real.
//
// @Summary      Import lớp và học sinh từ file Excel
// @Description  Đọc file .xlsx và tạo lớp, lịch học, phụ huynh, học sinh, ghi danh. dry_run=true chỉ kiểm tra, không ghi. Chỉ chủ trung tâm được gọi.
// @Tags         imports
// @Accept       multipart/form-data
// @Produce      json
// @Param        file     formData  file    true   "File .xlsx theo mẫu"
// @Param        dry_run  formData  bool    false  "true = chỉ kiểm tra, không ghi dữ liệu"
// @Success      200  {object}  response.Envelope{data=imports.Report}
// @Failure      400  {object}  response.Envelope
// @Failure      401  {object}  response.Envelope
// @Failure      403  {object}  response.Envelope
// @Failure      422  {object}  response.Envelope
// @Failure      429  {object}  response.Envelope
// @Security     BearerAuth
// @Router       /imports/roster [post]
func (h *Handler) importRoster(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	file, dryRun, appErr := readUpload(c)
	if appErr != nil {
		response.Err(c, appErr)
		return
	}
	rep, err := h.svc.Import(c.Request.Context(), sc, file, dryRun)
	if err != nil {
		var rowErrs *RowErrorsError
		if errors.As(err, &rowErrs) {
			// Fields' flat map cannot express sheet + line + column per defect.
			response.ErrWithDetails(c, rowErrs.AppError, rowErrs.Payload)
			return
		}
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, rep)
}

// readUpload reads the workbook out of a multipart request.
//
// The body is wrapped in MaxBytesReader BEFORE the form is parsed: gin buffers
// the whole upload (to memory, then to a temp file) as soon as FormFile is
// called, so a cap applied afterwards would protect nothing.
//
// The file's declared name is not trusted for anything. It is attacker
// controlled, so it decides no behaviour and is never echoed back into a
// response header; the parser validates the content by opening it.
func readUpload(c *gin.Context) ([]byte, bool, *apperror.AppError) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)

	header, err := c.FormFile("file")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return nil, false, apperror.BadRequest("file vượt quá 2 MB")
		}
		return nil, false, apperror.BadRequest("thiếu file trong trường \"file\"")
	}
	if header.Size > maxUploadBytes {
		return nil, false, apperror.BadRequest("file vượt quá 2 MB")
	}

	f, err := header.Open()
	if err != nil {
		return nil, false, apperror.BadRequest("không mở được file tải lên")
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, maxUploadBytes+1))
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return nil, false, apperror.BadRequest("file vượt quá 2 MB")
		}
		return nil, false, apperror.BadRequest("không đọc được file tải lên")
	}
	if len(data) > maxUploadBytes {
		return nil, false, apperror.BadRequest("file vượt quá 2 MB")
	}
	return data, parseDryRun(c.PostForm("dry_run")), nil
}

// parseDryRun reads the dry_run field. It defaults to true: a missing or
// malformed flag must not be read as "write to the database".
func parseDryRun(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "0", "no":
		return false
	default:
		return true
	}
}
