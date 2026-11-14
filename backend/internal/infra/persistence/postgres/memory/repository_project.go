package memory

import (
	"context"
	"errors"

	domainmemory "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/memory"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/dberror"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetMemoryProjectID 查询本人项目，不限制归档状态。
func (r *Repo) GetMemoryProjectID(ctx context.Context, userID uint, publicID string) (uint, error) {
	var project models.ConversationProject
	err := r.db.WithContext(ctx).Select("id").Where("user_id = ? AND public_id = ?", userID, publicID).First(&project).Error
	return project.ID, dberror.Translate(err)
}

// ListProjectMemories 按最近更新时间读取项目记忆。
func (r *Repo) ListProjectMemories(ctx context.Context, projectID uint) ([]domainmemory.ProjectMemory, error) {
	items := make([]models.ProjectMemory, 0)
	if err := r.db.WithContext(ctx).Where("project_id = ?", projectID).Order("updated_at DESC, id DESC").Find(&items).Error; err != nil {
		return nil, dberror.Translate(err)
	}
	result := make([]domainmemory.ProjectMemory, 0, len(items))
	for _, item := range items {
		result = append(result, projectMemoryDomain(item))
	}
	return result, nil
}

// UpsertProjectMemory 按项目和键写入，自动抽取不覆盖人工编辑。
func (r *Repo) UpsertProjectMemory(ctx context.Context, item *domainmemory.ProjectMemory) error {
	return dberror.Translate(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.ProjectMemory
		err := tx.Where("project_id = ? AND memory_key = ?", item.ProjectID, item.MemoryKey).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var count int64
			if err := tx.Model(&models.ProjectMemory{}).Where("project_id = ?", item.ProjectID).Count(&count).Error; err != nil {
				return err
			}
			if count >= 200 {
				return repository.ErrProjectMemoryLimitExceeded
			}
		} else if err != nil {
			return err
		}
		record := models.ProjectMemory{ProjectID: item.ProjectID, MemoryKey: item.MemoryKey, Value: item.Value, Source: item.Source}
		conflict := clause.OnConflict{
			Columns:   []clause.Column{{Name: "project_id"}, {Name: "memory_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "source", "updated_at"}),
		}
		if item.Source == "auto" {
			conflict.Where = clause.Where{Exprs: []clause.Expression{clause.Neq{Column: clause.Column{Table: "chat_conversation_project_memories", Name: "source"}, Value: "user"}}}
		}
		if err := tx.Clauses(conflict).Create(&record).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ? AND memory_key = ?", item.ProjectID, item.MemoryKey).First(&record).Error; err != nil {
			return err
		}
		*item = projectMemoryDomain(record)
		return nil
	}))
}

// DeleteProjectMemory 只删除指定项目内的记忆。
func (r *Repo) DeleteProjectMemory(ctx context.Context, projectID uint, memoryID uint) error {
	result := r.db.WithContext(ctx).Where("project_id = ? AND id = ?", projectID, memoryID).Delete(&models.ProjectMemory{})
	if result.Error != nil {
		return dberror.Translate(result.Error)
	}
	if result.RowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func projectMemoryDomain(item models.ProjectMemory) domainmemory.ProjectMemory {
	return domainmemory.ProjectMemory{ID: item.ID, ProjectID: item.ProjectID, MemoryKey: item.MemoryKey, Value: item.Value, Source: item.Source, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

func (r *Repo) DeleteProjectMemoryByKey(ctx context.Context, projectID uint, key string) error {
	result := r.db.WithContext(ctx).Where("project_id = ? AND memory_key = ?", projectID, key).Delete(&models.ProjectMemory{})
	if result.Error != nil {
		return dberror.Translate(result.Error)
	}
	if result.RowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}
