package memory

import (
	"errors"
	"net/http"
	"strconv"

	appmemory "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/memory"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

// ListProjectMemories godoc
// @Summary 查询项目共享记忆
// @Tags memory
// @Security BearerAuth
// @Produce json
// @Param id path string true "项目公开ID"
// @Success 200 {object} ProjectMemoryListResponseDoc
// @Failure 404 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /conversation-projects/{id}/memories [get]
func (h *Handler) ListProjectMemories(c *gin.Context) {
	items, err := h.service.ListProjectMemories(c.Request.Context(), middleware.MustUserID(c), c.Param("id"))
	if err != nil {
		writeProjectMemoryError(c, err)
		return
	}
	result := make([]ProjectMemoryResponse, 0, len(items))
	for _, item := range items {
		result = append(result, toProjectMemoryResponse(item))
	}
	response.Success(c, result)
}

// UpsertProjectMemory godoc
// @Summary 新建或更新项目共享记忆
// @Tags memory
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "项目公开ID"
// @Param body body UpsertProjectMemoryRequest true "记忆键与内容"
// @Success 200 {object} ProjectMemoryResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 404 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /conversation-projects/{id}/memories [put]
func (h *Handler) UpsertProjectMemory(c *gin.Context) {
	var req UpsertProjectMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	item, err := h.service.UpsertProjectMemory(c.Request.Context(), middleware.MustUserID(c), c.Param("id"), req.MemoryKey, req.Value, "user")
	if err != nil {
		writeProjectMemoryError(c, err)
		return
	}
	response.Success(c, ProjectMemoryDataResponse{Memory: toProjectMemoryResponse(*item)})
}

// DeleteProjectMemory godoc
// @Summary 删除项目共享记忆
// @Tags memory
// @Security BearerAuth
// @Produce json
// @Param id path string true "项目公开ID"
// @Param memoryID path int true "记忆ID"
// @Success 200 {object} ProjectMemoryDeleteResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 404 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /conversation-projects/{id}/memories/{memoryID} [delete]
func (h *Handler) DeleteProjectMemory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("memoryID"), 10, strconv.IntSize)
	if err != nil || id == 0 {
		response.ErrorWithCode(c, http.StatusBadRequest, "request.invalid_id")
		return
	}
	if err := h.service.DeleteProjectMemory(c.Request.Context(), middleware.MustUserID(c), c.Param("id"), uint(id)); err != nil {
		writeProjectMemoryError(c, err)
		return
	}
	response.Success(c, ProjectMemoryDeleteDataResponse{Deleted: true})
}

func writeProjectMemoryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, appmemory.ErrProjectNotFound), errors.Is(err, appmemory.ErrProjectMemoryNotFound):
		response.ErrorFrom(c, http.StatusNotFound, err)
	case errors.Is(err, appmemory.ErrInvalidProjectMemory), errors.Is(err, appmemory.ErrProjectMemoryLimitExceeded):
		response.ErrorFrom(c, http.StatusBadRequest, err)
	default:
		response.InternalError(c, err)
	}
}
