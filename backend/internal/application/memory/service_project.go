package memory

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	domainmemory "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/memory"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

// ListProjectMemories 查询当前用户项目的共享记忆。
func (s *Service) ListProjectMemories(ctx context.Context, userID uint, publicID string) ([]domainmemory.ProjectMemory, error) {
	projectID, err := s.memoryProjectID(ctx, userID, publicID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListProjectMemories(ctx, projectID)
}

func (s *Service) ListProjectMemoriesByProjectID(ctx context.Context, projectID uint) ([]domainmemory.ProjectMemory, error) {
	return s.repo.ListProjectMemories(ctx, projectID)
}

// UpsertProjectMemory 统一页面和模型写入的业务约束。
func (s *Service) UpsertProjectMemory(ctx context.Context, userID uint, publicID, key, value, source string) (*domainmemory.ProjectMemory, error) {
	projectID, err := s.memoryProjectID(ctx, userID, publicID)
	if err != nil {
		return nil, err
	}
	return s.UpsertProjectMemoryByProjectID(ctx, projectID, key, value, source)
}

func (s *Service) UpsertProjectMemoryByProjectID(ctx context.Context, projectID uint, key, value, source string) (*domainmemory.ProjectMemory, error) {
	key, value = strings.TrimSpace(key), strings.TrimSpace(value)
	if key == "" || utf8.RuneCountInString(key) > 128 || value == "" || utf8.RuneCountInString(value) > 2000 {
		return nil, ErrInvalidProjectMemory
	}
	item := &domainmemory.ProjectMemory{ProjectID: projectID, MemoryKey: key, Value: value, Source: source}
	if err := s.repo.UpsertProjectMemory(ctx, item); err != nil {
		if errors.Is(err, repository.ErrProjectMemoryLimitExceeded) {
			return nil, ErrProjectMemoryLimitExceeded
		}
		return nil, err
	}
	return item, nil
}

// DeleteProjectMemory 硬删除本人项目中的指定记忆。
func (s *Service) DeleteProjectMemory(ctx context.Context, userID uint, publicID string, memoryID uint) error {
	projectID, err := s.memoryProjectID(ctx, userID, publicID)
	if err != nil {
		return err
	}
	err = s.repo.DeleteProjectMemory(ctx, projectID, memoryID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrProjectMemoryNotFound
	}
	return err
}

func (s *Service) memoryProjectID(ctx context.Context, userID uint, publicID string) (uint, error) {
	id, err := s.repo.GetMemoryProjectID(ctx, userID, strings.TrimSpace(publicID))
	if errors.Is(err, repository.ErrNotFound) {
		return 0, ErrProjectNotFound
	}
	return id, err
}

func (s *Service) DeleteProjectMemoryByKey(ctx context.Context, projectID uint, key string) error {
	err := s.repo.DeleteProjectMemoryByKey(ctx, projectID, strings.TrimSpace(key))
	if errors.Is(err, repository.ErrNotFound) {
		return ErrProjectMemoryNotFound
	}
	return err
}
