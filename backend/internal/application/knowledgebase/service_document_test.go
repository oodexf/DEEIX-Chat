package knowledgebase

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	appupload "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/upload"
	domainconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	domainknowledgebase "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/knowledgebase"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"go.uber.org/zap"
)

func TestTextDocumentReadAndValidation(t *testing.T) {
	for _, test := range []struct {
		name, mimeType, text, title, body string
		wantErr                           error
	}{
		{name: "markdown", mimeType: "text/markdown", text: "# 标题\n\n正文\n", title: "标题", body: "正文\n"},
		{name: "plain", mimeType: "text/plain; charset=utf-8", text: "plain body\n", title: "notes", body: "plain body\n"},
		{name: "invalid UTF8", mimeType: "text/plain", text: string([]byte{255}), wantErr: ErrDocumentNotEditable},
		{name: "pdf", mimeType: "application/pdf", text: "body", wantErr: ErrDocumentNotEditable},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &knowledgeBaseRepositoryStub{item: &domainknowledgebase.KnowledgeBase{ID: 1, Scope: "user", OwnerUserID: 7}, file: &domainconversation.FileObject{FileID: "file", FileName: "notes.txt", UserID: 7, MimeType: test.mimeType}}
			service := NewService(repo)
			service.SetFileContentOpener(&knowledgeBaseFileOpenerStub{result: &appupload.FileContentResult{Reader: io.NopCloser(strings.NewReader(test.text))}})
			doc, err := service.ReadUserTextDocument(context.Background(), 7, "kb", "file")
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error: %v, want %v", err, test.wantErr)
			}
			if err == nil && (doc.Title != test.title || doc.Content != test.body || doc.FileID != "file") {
				t.Fatalf("document: %+v", doc)
			}
		})
	}
}

func TestMarkEditableDocumentsUsesOnlyMIME(t *testing.T) {
	files := []domainconversation.FileObject{
		{MimeType: "text/plain"}, {MimeType: "text/markdown; charset=utf-8"}, {MimeType: "application/pdf"},
	}
	markEditableDocuments(files)
	if !files[0].Editable || !files[1].Editable || files[2].Editable {
		t.Fatalf("editable flags: %+v", files)
	}
}

func TestTextDocumentWritesUseUploadAndSafeCleanup(t *testing.T) {
	for _, fileID := range []string{"", "old", "new"} {
		t.Run("replace_"+fileID, func(t *testing.T) {
			repo := &knowledgeBaseRepositoryStub{item: &domainknowledgebase.KnowledgeBase{ID: 1, Scope: "user", OwnerUserID: 7}, file: &domainconversation.FileObject{FileID: fileID, UserID: 7, MimeType: "text/markdown"}}
			uploader := &knowledgeBaseFileUploaderStub{result: &appupload.UploadFileResult{File: domainconversation.FileObject{FileID: "new", UserID: 7}}}
			cleaner := &knowledgeBaseFileCleanerStub{deleted: map[string]bool{}}
			service := NewService(repo)
			service.SetFileUploader(uploader)
			service.SetFileCleaner(cleaner)
			service.SetLogger(zap.NewNop())
			item, err := service.WriteUserTextDocument(context.Background(), 7, "kb", fileID, "新标题", "完整正文\n")
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(uploader.input.Reader)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != "# 新标题\n\n完整正文\n" || uploader.input.UserID != 7 || uploader.input.Purpose != "knowledge_base" || uploader.input.MimeType != "text/markdown" || uploader.input.MaxBytes != 0 {
				t.Fatalf("upload: %+v, %q", uploader.input, data)
			}
			if item.FileID != "new" || !item.Editable {
				t.Fatalf("result: %+v", item)
			}
			wantClean := 0
			if fileID == "old" {
				wantClean = 1
			}
			if cleaner.calls != wantClean {
				t.Fatalf("cleanup calls = %d, want %d", cleaner.calls, wantClean)
			}
		})
	}
}

func TestTextDocumentsRejectForeignBuiltinAndMissingFiles(t *testing.T) {
	for _, base := range []domainknowledgebase.KnowledgeBase{{ID: 1, Scope: "builtin"}, {ID: 2, Scope: "user", OwnerUserID: 8}} {
		repo := &knowledgeBaseRepositoryStub{item: &base}
		service := NewService(repo)
		if _, err := service.ReadUserTextDocument(context.Background(), 7, "kb", "file"); !errors.Is(err, ErrKnowledgeBaseNotFound) {
			t.Fatalf("read access: %v", err)
		}
		if _, err := service.WriteUserTextDocument(context.Background(), 7, "kb", "", "title", "content"); !errors.Is(err, ErrKnowledgeBaseNotFound) {
			t.Fatalf("write access: %v", err)
		}
	}
	repo := &knowledgeBaseRepositoryStub{item: &domainknowledgebase.KnowledgeBase{ID: 1, Scope: "user", OwnerUserID: 7}, fileErr: repository.ErrNotFound}
	service := NewService(repo)
	if _, err := service.WriteUserTextDocument(context.Background(), 7, "kb", "stale", "title", "content"); !errors.Is(err, ErrKnowledgeBaseFileNotFound) {
		t.Fatalf("stale file: %v", err)
	}
	for _, title := range []string{"", "two\nlines", strings.Repeat("字", 256)} {
		if _, err := service.WriteUserTextDocument(context.Background(), 7, "kb", "", title, "content"); !errors.Is(err, ErrInvalidKnowledgeBase) {
			t.Fatalf("invalid title: %v", err)
		}
	}
}
