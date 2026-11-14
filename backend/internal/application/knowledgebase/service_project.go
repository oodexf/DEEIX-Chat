package knowledgebase

import (
	"context"

	domainknowledgebase "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/knowledgebase"
)

func (s *Service) CreateUserForProject(ctx context.Context, userID uint, projectID uint, name, description string) (*domainknowledgebase.KnowledgeBase, error) {
	item, err := normalizeWriteInput(WriteInput{Name: name, Description: description, Enabled: true}, domainknowledgebase.ScopeUser, userID, userID)
	if err != nil {
		return nil, err
	}
	result, err := s.repo.CreateProjectKnowledgeBase(ctx, projectID, item)
	return result, mapRepositoryError(err)
}

func (s *Service) UpdateUserDetails(ctx context.Context, userID uint, publicID string, name, description *string) (*domainknowledgebase.KnowledgeBase, error) {
	return s.UpdateUser(ctx, userID, publicID, PatchInput{Name: name, Description: description})
}
