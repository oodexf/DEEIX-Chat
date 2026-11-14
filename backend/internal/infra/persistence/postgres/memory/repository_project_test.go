package memory

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	domainmemory "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/memory"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestProjectMemoryIsolationUpsertAndHardDelete(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "memories.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ConversationProject{}, &models.ProjectMemory{}); err != nil {
		t.Fatal(err)
	}
	projects := []models.ConversationProject{{UserID: 1, PublicID: "project-a", Status: "archived"}, {UserID: 2, PublicID: "project-b"}}
	if err := db.Create(&projects).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(db)
	ctx := context.Background()
	if _, err := repo.GetMemoryProjectID(ctx, 2, "project-a"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("cross-user access: %v", err)
	}
	if id, err := repo.GetMemoryProjectID(ctx, 1, "project-a"); err != nil || id != projects[0].ID {
		t.Fatalf("archived project: %d, %v", id, err)
	}
	a := &domainmemory.ProjectMemory{ProjectID: projects[0].ID, MemoryKey: "runtime", Value: "Go", Source: "user"}
	b := &domainmemory.ProjectMemory{ProjectID: projects[1].ID, MemoryKey: "runtime", Value: "Node", Source: "assistant"}
	for _, item := range []*domainmemory.ProjectMemory{a, b} {
		if err := repo.UpsertProjectMemory(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	auto := &domainmemory.ProjectMemory{ProjectID: a.ProjectID, MemoryKey: a.MemoryKey, Value: "Python", Source: "auto"}
	if err := repo.UpsertProjectMemory(ctx, auto); err != nil {
		t.Fatal(err)
	}
	if auto.Value != "Go" || auto.Source != "user" || auto.ID != a.ID {
		t.Fatalf("auto overwrote manual memory: %+v", auto)
	}
	b.Value, b.Source = "Rust", "auto"
	if err := repo.UpsertProjectMemory(ctx, b); err != nil {
		t.Fatal(err)
	}
	if b.Value != "Rust" {
		t.Fatalf("auto must update assistant memory: %+v", b)
	}
	if err := repo.DeleteProjectMemory(ctx, b.ProjectID, a.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("cross-project delete: %v", err)
	}
	if err := repo.DeleteProjectMemory(ctx, a.ProjectID, a.ID); err != nil {
		t.Fatal(err)
	}
	recreated := &domainmemory.ProjectMemory{ProjectID: a.ProjectID, MemoryKey: a.MemoryKey, Value: "C", Source: "user"}
	if err := repo.UpsertProjectMemory(ctx, recreated); err != nil {
		t.Fatal(err)
	}
	items, err := repo.ListProjectMemories(ctx, a.ProjectID)
	if err != nil || len(items) != 1 || items[0].Value != "C" {
		t.Fatalf("project list: %+v, %v", items, err)
	}
	var count int64
	if err := db.Unscoped().Model(&models.ProjectMemory{}).Where("id = ?", a.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("not hard deleted: %d, %v", count, err)
	}
}

func TestProjectMemoryLimitAndOrder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "limit.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ProjectMemory{}); err != nil {
		t.Fatal(err)
	}
	timestamp := time.Now().Add(-time.Hour)
	records := make([]models.ProjectMemory, 200)
	for i := range records {
		records[i] = models.ProjectMemory{ControlPlaneModel: models.ControlPlaneModel{UpdatedAt: timestamp}, ProjectID: 1, MemoryKey: fmt.Sprint(i), Value: "value", Source: "auto"}
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(db)
	ctx := context.Background()
	item := &domainmemory.ProjectMemory{ProjectID: 1, MemoryKey: "new", Value: "value", Source: "user"}
	if err := repo.UpsertProjectMemory(ctx, item); !errors.Is(err, repository.ErrProjectMemoryLimitExceeded) {
		t.Fatalf("limit error: %v", err)
	}
	item.MemoryKey = "0"
	if err := repo.UpsertProjectMemory(ctx, item); err != nil {
		t.Fatalf("existing key at limit: %v", err)
	}
	items, err := repo.ListProjectMemories(ctx, 1)
	if err != nil || len(items) != 200 || items[0].MemoryKey != "0" || items[1].MemoryKey != "199" {
		t.Fatalf("order: %+v, %v", items, err)
	}
}
