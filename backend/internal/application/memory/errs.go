package memory

import (
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/apperr"
)

var (
	// ErrUserMemoryLimitExceeded 用户记忆条数超限。
	ErrUserMemoryLimitExceeded = repository.ErrUserMemoryLimitExceeded

	ErrProjectNotFound            = apperr.New("conversation_project.not_found", "conversation project not found")
	ErrProjectMemoryNotFound      = apperr.New("memory.not_found", "memory not found")
	ErrInvalidProjectMemory       = apperr.New("request.invalid", "invalid project memory")
	ErrProjectMemoryLimitExceeded = apperr.New("memory.limit_exceeded", "project memory limit exceeded")
)
