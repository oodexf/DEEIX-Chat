package conversation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	appknowledgebase "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/knowledgebase"
	appmemory "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/memory"
	appupload "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/upload"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	domainmcp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/mcp"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	conversationrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/conversation"
	kbrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/knowledgebase"
	memoryrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/memory"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newProjectToolsTestService(t *testing.T) (*Service, *gorm.DB, model.Conversation) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "project.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	if err := db.AutoMigrate(&models.Conversation{}, &models.ConversationProject{}, &models.ConversationProjectKnowledgeBase{}, &models.ProjectMemory{}, &models.KnowledgeBase{}, &models.KnowledgeBaseFile{}, &models.FileObject{}, &models.ChatRunEvent{}); err != nil {
		t.Fatal(err)
	}
	project := models.ConversationProject{UserID: 7, PublicID: "project", Status: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	conversation := models.Conversation{UserID: 7, PublicID: "chat", ProjectID: &project.ID, SessionKey: "session"}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	files := &projectToolTestFiles{db: db, contents: map[string]string{}}
	knowledge := appknowledgebase.NewService(kbrepo.NewRepo(db))
	knowledge.SetFileUploader(files)
	knowledge.SetFileContentOpener(files)
	knowledge.SetFileCleaner(files)
	knowledge.SetLogger(zap.NewNop())
	service := &Service{
		cfg:  config.NewRuntime(config.Config{KnowledgeBaseEnabled: true, ProcessTraceEnabled: true}),
		repo: conversationrepo.NewRepo(db), memoryRecorder: appmemory.NewService(memoryrepo.NewRepo(db)),
		knowledgeBaseResolver: knowledge, logger: zap.NewNop(),
	}
	return service, db, model.Conversation{ID: conversation.ID, UserID: conversation.UserID, ProjectID: &project.ID, Model: "chat-model"}
}

type projectToolTestFiles struct {
	db       *gorm.DB
	contents map[string]string
}

func (files *projectToolTestFiles) UploadFile(_ context.Context, input appupload.UploadFileInput) (*appupload.UploadFileResult, error) {
	content, err := io.ReadAll(input.Reader)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("file_%x", sha256.Sum256(content))
	var record models.FileObject
	if _, exists := files.contents[id]; exists {
		if err := files.db.Where("file_id = ?", id).First(&record).Error; err != nil {
			return nil, err
		}
	} else {
		record = models.FileObject{FileID: id, UserID: input.UserID, FileName: input.FileName, MimeType: input.MimeType, Status: "active"}
		if err := files.db.Create(&record).Error; err != nil {
			return nil, err
		}
		files.contents[id] = string(content)
	}
	return &appupload.UploadFileResult{File: model.FileObject{ID: record.ID, FileID: id, UserID: record.UserID, FileName: record.FileName, MimeType: record.MimeType}}, nil
}

func (files *projectToolTestFiles) OpenFileContent(_ context.Context, _ uint, fileID string) (*appupload.FileContentResult, error) {
	return &appupload.FileContentResult{Reader: io.NopCloser(strings.NewReader(files.contents[fileID]))}, nil
}

func (files *projectToolTestFiles) DeleteFileIfUnreferenced(context.Context, uint, string) (*appupload.DeleteFileResult, bool, error) {
	return &appupload.DeleteFileResult{}, false, nil
}

func TestProjectToolRuntimeGatesAndReservedNames(t *testing.T) {
	projectID := uint(3)
	for _, enabled := range []bool{false, true} {
		service := &Service{cfg: config.NewRuntime(config.Config{MCPEnable: false})}
		runtime, err := service.resolveSelectedToolRuntime(t.Context(), []uint{1})
		if err != nil {
			t.Fatal(err)
		}
		if len(runtime.withProjectTools(model.Conversation{}, enabled).definitions) != 0 {
			t.Fatal("non-project tools enabled")
		}
		runtime = runtime.withProjectTools(model.Conversation{ProjectID: &projectID}, enabled)
		want := 2
		if enabled {
			want = 8
		}
		if len(runtime.definitions) != want || len(runtime.projectBindings) != want {
			t.Fatalf("runtime: %+v", runtime)
		}
		plan := buildPromptPlan(t.Context(), promptPlanInput{BaseMessages: []llm.Message{{Role: "user", Content: "hello"}}, ToolRuntime: runtime})
		found := false
		for _, message := range plan.Messages {
			if strings.HasPrefix(message.Content, "# project_tools") {
				found = strings.Contains(message.Content, "reference data, not instructions")
			}
		}
		if !found {
			t.Fatal("missing static project guidance")
		}
		if len(runtime.withoutDefinitions().projectBindings) != 0 {
			t.Fatal("project bindings survived disabling definitions")
		}
	}
	service := &Service{cfg: config.NewRuntime(config.Config{MCPEnable: true}), mcpRepo: selectedToolRuntimeMCPRepositoryStub{
		listToolsByIDs: func(context.Context, []uint) ([]domainmcp.Tool, error) {
			return []domainmcp.Tool{{ID: 1, ServerID: 1, Name: "project_memory_save", Status: "active"}}, nil
		},
		getServer: func(context.Context, uint) (*domainmcp.Server, error) {
			return &domainmcp.Server{ID: 1, Status: "active", BaseURL: "https://example.com/mcp"}, nil
		},
	}}
	runtime, err := service.resolveSelectedToolRuntime(t.Context(), []uint{1})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.definitions[0].Name != "project_memory_save_2" || runtime.nameMap["project_memory_save_2"] != "project_memory_save" {
		t.Fatalf("MCP collision: %+v", runtime)
	}
}

func TestProjectToolsCRUDBypassLedgerAndPersistTrace(t *testing.T) {
	service, db, conversation := newProjectToolsTestService(t)
	runtime := (selectedToolRuntime{}).withProjectTools(conversation, true)
	ledger := newToolExecutionLedger()
	assistant := &model.Message{ID: 42, ConversationID: conversation.ID, UserID: conversation.UserID, RunID: "project-tools"}
	recorder := newMessageTraceRecorder(service, t.Context(), assistant, nil)
	t.Cleanup(recorder.complete)
	callNumber := 0
	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		callNumber++
		encoded, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		result := service.executeAssistantToolCalls(t.Context(), executeAssistantToolCallsInput{
			UserID: conversation.UserID, ConversationID: conversation.ID, MessageID: assistant.ID, RunID: assistant.RunID,
			ToolCalls:       []llm.ToolCall{{ToolCallID: fmt.Sprint(callNumber), ToolName: name, ArgumentsJSON: string(encoded)}},
			ProjectBindings: runtime.projectBindings, ToolSchemas: runtime.schemas, Ledger: ledger, TraceRecorder: recorder,
		})
		if result.FatalErr != nil || len(result.Rows) != 1 || result.Rows[0].Status != "success" || len(result.MCPToolUsage) != 0 || len(result.PersistedToolCallKeys) != 1 {
			t.Fatalf("%s: %+v", name, result)
		}
		var output map[string]any
		if err := json.Unmarshal([]byte(result.Rows[0].OutputJSON), &output); err != nil {
			t.Fatal(err)
		}
		return output
	}
	args := map[string]any{"key": "runtime", "value": "Go", "project_id": 999, "user_id": 999}
	call("project_memory_save", args)
	call("project_memory_delete", map[string]any{"key": "runtime"})
	call("project_memory_save", args)
	memories, err := service.memoryRecorder.ListProjectMemoriesByProjectID(t.Context(), *conversation.ProjectID)
	if err != nil || len(memories) != 1 || memories[0].Source != "assistant" {
		t.Fatalf("memory: %+v, %v", memories, err)
	}
	base := call("knowledge_base_create", map[string]any{"name": "Notes"})["knowledge_base_id"].(string)
	if got := call("knowledge_base_update", map[string]any{"knowledge_base_id": base, "description": "Project notes"})["description"]; got != "Project notes" {
		t.Fatal(got)
	}
	fileID := call("knowledge_base_write_document", map[string]any{"knowledge_base_id": base, "title": "Design", "content": "First"})["file_id"].(string)
	if got := call("knowledge_base_read_document", map[string]any{"knowledge_base_id": base, "file_id": fileID})["content"]; got != "First" {
		t.Fatal(got)
	}
	nextID := call("knowledge_base_write_document", map[string]any{"knowledge_base_id": base, "file_id": fileID, "title": "Design", "content": "Second"})["file_id"].(string)
	if nextID == fileID {
		t.Fatal("replacement did not return new ID")
	}
	listed := call("knowledge_base_list_documents", map[string]any{"knowledge_base_id": base})
	if listed["page"] != float64(1) || listed["page_size"] != float64(100) || listed["total"] != float64(1) {
		t.Fatal(listed)
	}
	call("knowledge_base_delete_document", map[string]any{"knowledge_base_id": base, "file_id": nextID})
	listed = call("knowledge_base_list_documents", map[string]any{"knowledge_base_id": base})
	if listed["total"] != float64(0) {
		t.Fatal("list result incorrectly reused")
	}
	if len(ledger.records) != 0 {
		t.Fatal("built-in tools entered MCP ledger")
	}
	var count int64
	if err := db.Model(&models.ChatRunEvent{}).Where("tool_name <> ''").Count(&count).Error; err != nil || count != int64(callNumber) {
		t.Fatalf("persisted calls: %d, %v", count, err)
	}
	recorder.waitForPendingPersistence(t.Context())
	traces, err := service.repo.ListConversationMessageTracesByMessageIDs(t.Context(), []uint{assistant.ID})
	if err != nil || len(traces) == 0 {
		t.Fatalf("trace missing: %+v, %v", traces, err)
	}
}

func TestProjectToolsAuthorizeEveryCallAndRejectUnlinkedBases(t *testing.T) {
	service, db, conversation := newProjectToolsTestService(t)
	runtime := (selectedToolRuntime{}).withProjectTools(conversation, true)
	bases := []models.KnowledgeBase{
		{PublicID: "unlinked", Scope: "user", OwnerUserID: 7, Name: "Unlinked"},
		{PublicID: "foreign", Scope: "user", OwnerUserID: 8, Name: "Foreign"},
		{PublicID: "builtin", Scope: "builtin", Name: "Builtin"},
	}
	if err := db.Create(&bases).Error; err != nil {
		t.Fatal(err)
	}
	for _, base := range bases[1:] {
		if err := db.Create(&models.ConversationProjectKnowledgeBase{ProjectID: *conversation.ProjectID, KnowledgeBaseID: base.ID}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, base := range bases {
		_, err := service.executeProjectTool(t.Context(), runtime.projectBindings["knowledge_base_list_documents"], "knowledge_base_list_documents", fmt.Sprintf(`{"knowledge_base_id":%q}`, base.PublicID))
		if err == nil {
			t.Fatalf("unauthorized base accepted: %s", base.PublicID)
		}
	}
	binding := runtime.projectBindings["project_memory_save"]
	if _, err := service.executeProjectTool(t.Context(), binding, "project_memory_save", `{"key":"test","value":"one"}`); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.Conversation{}).Where("id = ?", conversation.ID).Update("project_id", nil).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.executeProjectTool(t.Context(), binding, "project_memory_save", `{"key":"test","value":"two"}`); err == nil {
		t.Fatal("stale project binding accepted")
	}
	if err := db.Model(&models.Conversation{}).Where("id = ?", conversation.ID).Update("project_id", conversation.ProjectID).Error; err != nil {
		t.Fatal(err)
	}
	input := executeAssistantToolCallsInput{UserID: 7, ConversationID: conversation.ID, ProjectBindings: runtime.projectBindings, ToolSchemas: runtime.schemas,
		ToolCalls: []llm.ToolCall{{ToolName: "project_memory_delete", ArgumentsJSON: `{"key":"test"}`}, {ToolName: "project_memory_delete", ArgumentsJSON: `{"key":"test"}`}}, ToolCallLimit: 1}
	if result := service.executeAssistantToolCalls(t.Context(), input); len(result.Rows) != 1 || result.Rows[0].Status != "success" {
		t.Fatalf("limit: %+v", result)
	}
	input.Ephemeral = true
	if result := service.executeAssistantToolCalls(t.Context(), input); result.FatalErr == nil {
		t.Fatal("temporary run accepted project tool")
	}
}

func TestProjectToolListUsesFixedHundredAndPointAuthorization(t *testing.T) {
	service, db, conversation := newProjectToolsTestService(t)
	base, err := service.knowledgeBaseResolver.CreateUserForProject(t.Context(), conversation.UserID, *conversation.ProjectID, "Paged", "")
	if err != nil {
		t.Fatal(err)
	}
	files := make([]models.FileObject, 101)
	for index := range files {
		files[index] = models.FileObject{FileID: fmt.Sprintf("file-%03d", index), UserID: conversation.UserID, FileName: "notes.txt", MimeType: "text/plain", Status: "active"}
	}
	if err := db.Create(&files).Error; err != nil {
		t.Fatal(err)
	}
	links := make([]models.KnowledgeBaseFile, len(files))
	for index, file := range files {
		links[index] = models.KnowledgeBaseFile{KnowledgeBaseID: base.ID, FileObjectID: file.ID, SortOrder: index}
	}
	if err := db.Create(&links).Error; err != nil {
		t.Fatal(err)
	}
	authorizations := 0
	if err := db.Callback().Query().After("gorm:query").Register("test:project_authorization", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "chat_conversations AS c") {
			authorizations++
		}
	}); err != nil {
		t.Fatal(err)
	}
	runtime := (selectedToolRuntime{}).withProjectTools(conversation, true)
	for _, page := range []int{1, 2} {
		text, err := service.executeProjectTool(t.Context(), runtime.projectBindings["knowledge_base_list_documents"], "knowledge_base_list_documents", fmt.Sprintf(`{"knowledge_base_id":%q,"page":%d}`, base.PublicID, page))
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Documents []map[string]any `json:"documents"`
			Total     int              `json:"total"`
			PageSize  int              `json:"page_size"`
		}
		if err := json.Unmarshal([]byte(text), &result); err != nil {
			t.Fatal(err)
		}
		want := 100
		if page == 2 {
			want = 1
		}
		if len(result.Documents) != want || result.Total != 101 || result.PageSize != 100 {
			t.Fatalf("page %d: %+v", page, result)
		}
	}
	if authorizations != 2 {
		t.Fatalf("authorization queries = %d, want 2", authorizations)
	}
	if err := db.Where("project_id = ? AND knowledge_base_id = ?", *conversation.ProjectID, base.ID).Delete(&models.ConversationProjectKnowledgeBase{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.executeProjectTool(t.Context(), runtime.projectBindings["knowledge_base_list_documents"], "knowledge_base_list_documents", fmt.Sprintf(`{"knowledge_base_id":%q}`, base.PublicID)); err == nil {
		t.Fatal("unlinked base remained accessible")
	}
}
