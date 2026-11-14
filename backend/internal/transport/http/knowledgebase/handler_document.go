package knowledgebase

import (
	"errors"
	"net/http"

	appconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/conversation"
	appknowledgebase "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/knowledgebase"
	domainconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

// CreateMineDocument godoc
// @Summary 在个人知识库中新建文本文档
// @Tags knowledge-bases
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "知识库公开ID"
// @Param body body WriteTextDocumentRequest true "标题和正文"
// @Success 200 {object} KnowledgeBaseFileResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 403 {object} ErrorDoc
// @Failure 404 {object} ErrorDoc
// @Failure 409 {object} ErrorDoc "存储配额不足"
// @Failure 413 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /knowledge-bases/mine/{id}/documents [post]
func (h *Handler) CreateMineDocument(c *gin.Context) { h.writeMineDocument(c, "") }

// UpdateMineDocument godoc
// @Summary 全量替换个人知识库文本文档
// @Description 返回实际文件 ID，内容变化后可能与原 ID 不同
// @Tags knowledge-bases
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "知识库公开ID"
// @Param fileID path string true "文件ID"
// @Param body body WriteTextDocumentRequest true "标题和完整正文"
// @Success 200 {object} KnowledgeBaseFileResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 403 {object} ErrorDoc
// @Failure 404 {object} ErrorDoc
// @Failure 409 {object} ErrorDoc "存储配额不足"
// @Failure 413 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /knowledge-bases/mine/{id}/documents/{fileID} [put]
func (h *Handler) UpdateMineDocument(c *gin.Context) { h.writeMineDocument(c, c.Param("fileID")) }

// GetMineDocument godoc
// @Summary 读取个人知识库文本文档的标题和正文
// @Tags knowledge-bases
// @Security BearerAuth
// @Produce json
// @Param id path string true "知识库公开ID"
// @Param fileID path string true "文件ID"
// @Success 200 {object} TextDocumentResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 403 {object} ErrorDoc
// @Failure 404 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /knowledge-bases/mine/{id}/documents/{fileID} [get]
func (h *Handler) GetMineDocument(c *gin.Context) {
	item, err := h.service.ReadUserTextDocument(c.Request.Context(), middleware.MustUserID(c), c.Param("id"), c.Param("fileID"))
	if err != nil {
		writeDocumentError(c, err)
		return
	}
	response.Success(c, TextDocumentDataResponse{Document: TextDocumentResponse{FileID: item.FileID, Title: item.Title, Content: item.Content}})
}

func (h *Handler) writeMineDocument(c *gin.Context, fileID string) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxUploadRequestBytes())
	var req WriteTextDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			response.ErrorFrom(c, http.StatusRequestEntityTooLarge, appconversation.ErrFileTooLarge)
		} else {
			response.InvalidRequestBody(c, err)
		}
		return
	}
	file, err := h.service.WriteUserTextDocument(c.Request.Context(), middleware.MustUserID(c), c.Param("id"), fileID, req.Title, req.Content)
	if err != nil {
		writeDocumentError(c, err)
		return
	}
	capabilities := h.service.ResolveFileVectorizationCapabilities(c.Request.Context(), []domainconversation.FileObject{*file})
	response.Success(c, KnowledgeBaseFileDataResponse{File: toKnowledgeBaseFileResponse(*file, capabilities[file.FileID])})
}

func writeDocumentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, appknowledgebase.ErrDocumentNotEditable):
		response.ErrorFrom(c, http.StatusBadRequest, err)
	case errors.Is(err, appconversation.ErrFileTooLarge):
		response.ErrorFrom(c, http.StatusRequestEntityTooLarge, err)
	case errors.Is(err, appconversation.ErrStorageQuotaExceeded):
		response.ErrorFrom(c, http.StatusConflict, errStorageQuotaExceeded)
	case errors.Is(err, appconversation.ErrDangerousMIMEType), errors.Is(err, appconversation.ErrMIMEBlocked), errors.Is(err, appconversation.ErrInvalidFileReference), errors.Is(err, appconversation.ErrInvalidFileName):
		response.ErrorFrom(c, http.StatusBadRequest, errInvalidFile)
	default:
		writeError(c, err)
	}
}
