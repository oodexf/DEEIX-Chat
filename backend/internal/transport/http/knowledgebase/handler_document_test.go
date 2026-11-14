package knowledgebase

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	appconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/conversation"
	appknowledgebase "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/knowledgebase"
	appupload "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/upload"
	domainconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	kbrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/knowledgebase"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// documentTestFiles 模拟内容去重的文件管道，关联读写使用真实 SQLite 仓储。
type documentTestFiles struct {
	db        *gorm.DB
	files     map[string]domainconversation.FileObject
	contents  map[string]string
	uploadErr error
}

func (f *documentTestFiles) UploadFile(_ context.Context, input appupload.UploadFileInput) (*appupload.UploadFileResult, error) {
	if f.uploadErr != nil {
		return nil, f.uploadErr
	}
	content, err := io.ReadAll(input.Reader)
	if err != nil {
		return nil, err
	}
	if len(content) > 256 {
		return nil, appconversation.ErrFileTooLarge
	}
	id := fmt.Sprintf("file_%x", sha256.Sum256(content))
	if item, ok := f.files[id]; ok {
		return &appupload.UploadFileResult{File: item, Reused: true}, nil
	}
	record := models.FileObject{FileID: id, UserID: input.UserID, FileName: input.FileName, MimeType: input.MimeType, Status: "active"}
	if err := f.db.Create(&record).Error; err != nil {
		return nil, err
	}
	item := domainconversation.FileObject{ID: record.ID, FileID: id, UserID: input.UserID, FileName: input.FileName, MimeType: input.MimeType, Status: "active"}
	f.files[id], f.contents[id] = item, string(content)
	return &appupload.UploadFileResult{File: item}, nil
}

func (f *documentTestFiles) OpenFileContent(_ context.Context, _ uint, fileID string) (*appupload.FileContentResult, error) {
	item, ok := f.files[fileID]
	if !ok {
		return nil, appconversation.ErrFileNotFound
	}
	return &appupload.FileContentResult{File: item, Reader: io.NopCloser(strings.NewReader(f.contents[fileID]))}, nil
}

func (f *documentTestFiles) DeleteFileIfUnreferenced(_ context.Context, _ uint, fileID string) (*appupload.DeleteFileResult, bool, error) {
	var count int64
	err := f.db.Model(&models.KnowledgeBaseFile{}).Where("file_object_id = ?", f.files[fileID].ID).Count(&count).Error
	if err != nil {
		return nil, false, err
	}
	if count != 0 {
		return nil, false, nil
	}
	return &appupload.DeleteFileResult{Deleted: true, FileID: fileID}, true, nil
}

func TestTextDocumentHTTPContract(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "http.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.KnowledgeBase{}, &models.KnowledgeBaseFile{}, &models.FileObject{}); err != nil {
		t.Fatal(err)
	}
	bases := []models.KnowledgeBase{{PublicID: "kb-mine", Scope: "user", OwnerUserID: 7, Name: "Mine"}, {PublicID: "other", Scope: "user", OwnerUserID: 8, Name: "Other"}, {PublicID: "builtin", Scope: "builtin", Name: "Builtin"}}
	if err := db.Create(&bases).Error; err != nil {
		t.Fatal(err)
	}
	files := &documentTestFiles{db: db, files: make(map[string]domainconversation.FileObject), contents: make(map[string]string)}
	service := appknowledgebase.NewService(kbrepo.NewRepo(db))
	service.SetFileUploader(files)
	service.SetFileContentOpener(files)
	service.SetFileCleaner(files)
	service.SetLogger(zap.NewNop())
	router := gin.New()
	group := router.Group("/api/v1", func(c *gin.Context) { c.Set(middleware.ContextKeyUserID, uint(7)) })
	NewModule(NewHandler(service, config.NewRuntime(config.Config{KnowledgeBaseEnabled: true, MaxUploadFileBytes: 256}))).RegisterRoutes(group)
	request := func(method, path, body string, status int) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	const base = "/knowledge-bases/mine/kb-mine/documents"
	saved := request(http.MethodPost, base, `{"title":"标题","content":"正文\n"}`, 200)
	file := saved["data"].(map[string]any)["file"].(map[string]any)
	if len(file) != 19 || file["editable"] != true {
		t.Fatalf("file DTO: %+v", file)
	}
	id := file["fileID"].(string)
	read := request(http.MethodGet, base+"/"+id, "", 200)["data"].(map[string]any)["document"].(map[string]any)
	if len(read) != 3 || read["title"] != "标题" || read["content"] != "正文\n" || read["fileID"] != id {
		t.Fatalf("document DTO: %+v", read)
	}
	unchanged := request(http.MethodPut, base+"/"+id, `{"title":"标题","content":"正文\n"}`, 200)
	if unchanged["data"].(map[string]any)["file"].(map[string]any)["fileID"] != id {
		t.Fatal("same content changed file ID")
	}
	updated := request(http.MethodPut, base+"/"+id, `{"title":"标题","content":"新正文"}`, 200)
	if updated["data"].(map[string]any)["file"].(map[string]any)["fileID"] == id {
		t.Fatal("changed content kept file ID")
	}
	if code := request(http.MethodPut, base+"/"+id, `{"title":"标题","content":"正文"}`, 404)["errorCode"]; code != "knowledge_base.not_found" {
		t.Fatal(code)
	}
	listed := request(http.MethodGet, "/knowledge-bases/kb-mine/files", "", 200)["data"].(map[string]any)["results"].([]any)
	if len(listed) != 1 || listed[0].(map[string]any)["editable"] != true {
		t.Fatalf("editable listing: %+v", listed)
	}
	for _, kb := range []string{"other", "builtin", "missing"} {
		if code := request(http.MethodPost, "/knowledge-bases/mine/"+kb+"/documents", `{"title":"标题","content":"正文"}`, 404)["errorCode"]; code != "knowledge_base.not_found" {
			t.Fatal(code)
		}
	}
	if code := request(http.MethodPost, base, `{"title":"","content":"x"}`, 400)["errorCode"]; code != "knowledge_base.invalid" {
		t.Fatal(code)
	}
	if code := request(http.MethodPost, base, `{"title":1}`, 400)["errorCode"]; code != "request.invalid_body" {
		t.Fatal(code)
	}
	if code := request(http.MethodPost, base, `{"title":"title","content":"`+strings.Repeat("x", 300)+`"}`, 413)["errorCode"]; code != "file.too_large" {
		t.Fatal(code)
	}
	if code := request(http.MethodPost, base, `{"title":"title","content":"`+strings.Repeat("x", (1<<20)+512)+`"}`, 413)["errorCode"]; code != "file.too_large" {
		t.Fatal(code)
	}
	files.uploadErr = appconversation.ErrStorageQuotaExceeded
	if code := request(http.MethodPost, base, `{"title":"title","content":"x"}`, 409)["errorCode"]; code != "quota.exceeded" {
		t.Fatal(code)
	}
	files.uploadErr = nil
	// 列表只判断文本格式，非 UTF-8 内容在实际读取时拒绝。
	invalid, err := files.UploadFile(context.Background(), appupload.UploadFileInput{UserID: 7, FileName: "legacy.txt", MimeType: "text/plain", Reader: strings.NewReader(string([]byte{255}))})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.AddUserFiles(context.Background(), 7, "kb-mine", []string{invalid.File.FileID}); err != nil {
		t.Fatal(err)
	}
	if code := request(http.MethodGet, base+"/"+invalid.File.FileID, "", 400)["errorCode"]; code != "knowledge_base.document_not_editable" {
		t.Fatal(code)
	}
	listed = request(http.MethodGet, "/knowledge-bases/kb-mine/files", "", 200)["data"].(map[string]any)["results"].([]any)
	for _, raw := range listed {
		item := raw.(map[string]any)
		if item["fileID"] == invalid.File.FileID && item["editable"] != true {
			t.Fatal("text MIME must expose editing")
		}
	}
}
