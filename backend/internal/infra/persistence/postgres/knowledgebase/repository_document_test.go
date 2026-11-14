package knowledgebase

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestReplaceTextFilePreservesSharedReferencesAndRevision(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "documents.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.KnowledgeBase{}, &models.KnowledgeBaseFile{}, &models.FileObject{}); err != nil {
		t.Fatal(err)
	}
	bases := []models.KnowledgeBase{{PublicID: "one", Name: "One", Scope: "user", OwnerUserID: 1}, {PublicID: "two", Name: "Two", Scope: "user", OwnerUserID: 1}}
	files := []models.FileObject{{FileID: "old", UserID: 1, Status: "active"}, {FileID: "new", UserID: 1, Status: "active"}, {FileID: "other", UserID: 2, Status: "active"}}
	if err := db.Create(&bases).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&files).Error; err != nil {
		t.Fatal(err)
	}
	links := []models.KnowledgeBaseFile{
		{KnowledgeBaseID: bases[0].ID, FileObjectID: files[0].ID, SortOrder: 7},
		{KnowledgeBaseID: bases[1].ID, FileObjectID: files[0].ID, SortOrder: 1},
		{KnowledgeBaseID: bases[0].ID, FileObjectID: files[1].ID, SortOrder: 8},
	}
	if err := db.Create(&links).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(db)
	ctx := context.Background()
	if err := repo.ReplaceKnowledgeBaseTextFile(ctx, bases[0].ID, "old", "other", 1); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("foreign file accepted: %v", err)
	}
	if _, err := repo.GetKnowledgeBaseFile(ctx, bases[0].ID, "old"); err != nil {
		t.Fatalf("failed update removed old link: %v", err)
	}
	if err := repo.ReplaceKnowledgeBaseTextFile(ctx, bases[0].ID, "old", "old", 1); err != nil {
		t.Fatalf("same content: %v", err)
	}
	if err := repo.ReplaceKnowledgeBaseTextFile(ctx, bases[0].ID, "old", "new", 1); err != nil {
		t.Fatalf("already linked target: %v", err)
	}
	if _, err := repo.GetKnowledgeBaseFile(ctx, bases[1].ID, "old"); err != nil {
		t.Fatalf("shared reference changed: %v", err)
	}
	if _, err := repo.GetKnowledgeBaseFile(ctx, bases[0].ID, "old"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("old link still exists: %v", err)
	}
	if err := repo.ReplaceKnowledgeBaseTextFile(ctx, bases[0].ID, "old", "new", 1); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("stale update: %v", err)
	}
	var base models.KnowledgeBase
	if err := db.First(&base, bases[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if base.Revision != 2 {
		t.Fatalf("revision = %d, want 2", base.Revision)
	}
	var count int64
	if err := db.Model(&models.KnowledgeBaseFile{}).Where("knowledge_base_id = ?", base.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("target links = %d, %v", count, err)
	}
	if err := db.Model(&models.FileObject{}).Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("file objects changed: %d, %v", count, err)
	}
	// 未关联的新目标应继承文档排序位置。
	if err := db.Where("knowledge_base_id = ? AND file_object_id = ?", bases[0].ID, files[1].ID).Delete(&models.KnowledgeBaseFile{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.KnowledgeBaseFile{KnowledgeBaseID: bases[0].ID, FileObjectID: files[0].ID, SortOrder: 7}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceKnowledgeBaseTextFile(ctx, bases[0].ID, "old", "new", 1); err != nil {
		t.Fatal(err)
	}
	var link models.KnowledgeBaseFile
	if err := db.Where("knowledge_base_id = ?", bases[0].ID).First(&link).Error; err != nil || link.SortOrder != 7 {
		t.Fatalf("replacement order: %+v, %v", link, err)
	}
}
