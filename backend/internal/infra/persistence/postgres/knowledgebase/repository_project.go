package knowledgebase

import (
	"context"

	domainknowledgebase "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/knowledgebase"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/dberror"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"gorm.io/gorm"
)

// CreateProjectKnowledgeBase 在同一事务内建库并关联项目，空库无需等待检索就绪。
func (r *Repo) CreateProjectKnowledgeBase(ctx context.Context, projectID uint, item *domainknowledgebase.KnowledgeBase) (*domainknowledgebase.KnowledgeBase, error) {
	entity := toModel(item)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.ConversationProjectKnowledgeBase{}).Where("project_id = ?", projectID).Count(&count).Error; err != nil {
			return err
		}
		if count >= 8 {
			return repository.ErrInvalidInput
		}
		if err := tx.Create(&entity).Error; err != nil {
			return err
		}
		return tx.Create(&models.ConversationProjectKnowledgeBase{ProjectID: projectID, KnowledgeBaseID: entity.ID, SortOrder: int(count) + 1}).Error
	})
	if err != nil {
		return nil, dberror.Translate(err)
	}
	result := toDomain(entity)
	return &result, nil
}
