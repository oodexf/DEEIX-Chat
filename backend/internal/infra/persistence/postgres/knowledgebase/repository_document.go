package knowledgebase

import (
	"context"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/dberror"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReplaceKnowledgeBaseTextFile 原子替换文档关联，文件对象及其他库的引用不变。
func (r *Repo) ReplaceKnowledgeBaseTextFile(ctx context.Context, knowledgeBaseID uint, oldFileID, newFileID string, actorUserID uint) error {
	return dberror.Translate(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old models.KnowledgeBaseFile
		if err := tx.Where("knowledge_base_id = ? AND file_object_id IN (?)", knowledgeBaseID,
			tx.Model(&models.FileObject{}).Select("id").Where("file_id = ? AND status = ?", oldFileID, "active")).First(&old).Error; err != nil {
			return err
		}
		if oldFileID == newFileID {
			return nil
		}
		var file models.FileObject
		if err := tx.Where("file_id = ? AND user_id = ? AND status = ?", newFileID, actorUserID, "active").First(&file).Error; err != nil {
			return err
		}
		// 相同内容可能已在当前库内；复用已有目标关联即可。
		next := models.KnowledgeBaseFile{KnowledgeBaseID: knowledgeBaseID, FileObjectID: file.ID, SortOrder: old.SortOrder, AddedByUserID: actorUserID}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&next).Error; err != nil {
			return err
		}
		if err := tx.Delete(&old).Error; err != nil {
			return err
		}
		return tx.Model(&models.KnowledgeBase{}).Where("id = ?", knowledgeBaseID).Update("revision", gorm.Expr("revision + 1")).Error
	}))
}
