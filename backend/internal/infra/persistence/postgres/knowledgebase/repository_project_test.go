package knowledgebase

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	domainknowledgebase "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/knowledgebase"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateProjectKnowledgeBaseLimitAndRollback(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "project.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.KnowledgeBase{}, &models.ConversationProjectKnowledgeBase{}); err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(db)
	for index := 0; index < 9; index++ {
		name := fmt.Sprintf("base-%d", index)
		_, err := repo.CreateProjectKnowledgeBase(t.Context(), 1, &domainknowledgebase.KnowledgeBase{PublicID: name, Name: name, Scope: "user", OwnerUserID: 7, Enabled: true})
		if index < 8 && err != nil {
			t.Fatal(err)
		}
		if index == 8 && !errors.Is(err, repository.ErrInvalidInput) {
			t.Fatalf("ninth base: %v", err)
		}
	}
	var count int64
	if err := db.Model(&models.KnowledgeBase{}).Count(&count).Error; err != nil || count != 8 {
		t.Fatalf("base count: %d, %v", count, err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_project_link BEFORE INSERT ON chat_conversation_project_knowledge_bases BEGIN SELECT RAISE(ABORT, 'link failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateProjectKnowledgeBase(t.Context(), 2, &domainknowledgebase.KnowledgeBase{PublicID: "rollback", Name: "Rollback", Scope: "user", OwnerUserID: 7}); err == nil {
		t.Fatal("expected link failure")
	}
	if err := db.Model(&models.KnowledgeBase{}).Count(&count).Error; err != nil || count != 8 {
		t.Fatalf("orphan base after failure: %d, %v", count, err)
	}
}
