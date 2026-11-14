package memory

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	appmemory "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/memory"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	memoryrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/memory"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestProjectMemoryHTTPContract(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "http.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ConversationProject{}, &models.ProjectMemory{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]models.ConversationProject{{UserID: 1, PublicID: "mine", Status: "archived"}, {UserID: 2, PublicID: "other"}}).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	group := router.Group("/api/v1", func(c *gin.Context) { c.Set(middleware.ContextKeyUserID, uint(1)) })
	// 与现有项目路由共用通配符，避免注册时冲突。
	group.DELETE("/conversation-projects/:id", func(c *gin.Context) {})
	NewModule(NewHandler(appmemory.NewService(memoryrepo.NewRepo(db)))).RegisterRoutes(group)
	request := func(method, path, body string, status int) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		var response map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	empty := request(http.MethodGet, "/conversation-projects/mine/memories", "", 200)
	if len(empty["data"].([]any)) != 0 {
		t.Fatal(empty)
	}
	saved := request(http.MethodPut, "/conversation-projects/mine/memories", `{"memoryKey":"runtime","value":"Go"}`, 200)
	item := saved["data"].(map[string]any)["memory"].(map[string]any)
	if len(item) != 6 || item["source"] != "user" || item["memoryKey"] != "runtime" {
		t.Fatalf("memory DTO: %+v", item)
	}
	if code := request(http.MethodGet, "/conversation-projects/other/memories", "", 404)["errorCode"]; code != "conversation_project.not_found" {
		t.Fatal(code)
	}
	if code := request(http.MethodPut, "/conversation-projects/mine/memories", `{"memoryKey":"","value":"x"}`, 400)["errorCode"]; code != "request.invalid" {
		t.Fatal(code)
	}
	path := fmt.Sprintf("/conversation-projects/mine/memories/%.0f", item["id"])
	deleted := request(http.MethodDelete, path, "", 200)
	if deleted["data"].(map[string]any)["deleted"] != true {
		t.Fatal(deleted)
	}
	if code := request(http.MethodDelete, path, "", 404)["errorCode"]; code != "memory.not_found" {
		t.Fatal(code)
	}
}
