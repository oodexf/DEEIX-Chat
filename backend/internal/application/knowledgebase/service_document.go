package knowledgebase

import (
	"context"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"unicode/utf8"

	appupload "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/upload"
	domainconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	domainknowledgebase "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/knowledgebase"
	"go.uber.org/zap"
)

// WriteUserTextDocument 新建文档或全量替换已有文档，复用统一上传管道。
func (s *Service) WriteUserTextDocument(ctx context.Context, userID uint, publicID, fileID, title, content string) (*domainconversation.FileObject, error) {
	title = strings.TrimSpace(title)
	if title == "" || utf8.RuneCountInString(title) > 255 || strings.ContainsAny(title, "\r\n") {
		return nil, ErrInvalidKnowledgeBase
	}
	base, err := s.userDocumentBase(ctx, userID, publicID)
	if err != nil {
		return nil, err
	}
	if fileID != "" {
		file, err := s.repo.GetKnowledgeBaseFile(ctx, base.ID, fileID)
		if err != nil {
			return nil, mapFileRepositoryError(err)
		}
		if !isTextDocument(*file) {
			return nil, ErrDocumentNotEditable
		}
	}
	text := "# " + title + "\n\n" + content
	uploaded, err := s.fileUploader.UploadFile(ctx, appupload.UploadFileInput{
		UserID: userID, Purpose: "knowledge_base", FileName: title + ".md", MimeType: "text/markdown",
		DeclaredSize: int64(len(text)), Reader: strings.NewReader(text),
	})
	if err != nil {
		return nil, err
	}
	if fileID == "" {
		err = s.repo.AddKnowledgeBaseFiles(ctx, base.ID, domainknowledgebase.ScopeUser, userID, userID, []string{uploaded.File.FileID})
	} else {
		err = s.repo.ReplaceKnowledgeBaseTextFile(ctx, base.ID, fileID, uploaded.File.FileID, userID)
	}
	if err != nil {
		return nil, mapFileRepositoryError(err)
	}
	if fileID != "" && fileID != uploaded.File.FileID {
		if _, _, err := s.fileCleaner.DeleteFileIfUnreferenced(ctx, userID, fileID); err != nil {
			s.logger.Warn("knowledge_base_old_document_cleanup_failed", zap.String("file_id", fileID), zap.Error(err))
		}
	}
	uploaded.File.Editable = true
	return &uploaded.File, nil
}

// ReadUserTextDocument 读取本人个人库内的可编辑文本。
func (s *Service) ReadUserTextDocument(ctx context.Context, userID uint, publicID, fileID string) (*domainknowledgebase.TextDocument, error) {
	base, err := s.userDocumentBase(ctx, userID, publicID)
	if err != nil {
		return nil, err
	}
	return s.readTextDocument(ctx, base.ID, fileID)
}

func (s *Service) userDocumentBase(ctx context.Context, userID uint, publicID string) (*domainknowledgebase.KnowledgeBase, error) {
	item, err := s.getForAccess(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if item.Scope != domainknowledgebase.ScopeUser || item.OwnerUserID != userID {
		return nil, ErrKnowledgeBaseNotFound
	}
	return item, nil
}

func (s *Service) readTextDocument(ctx context.Context, knowledgeBaseID uint, fileID string) (*domainknowledgebase.TextDocument, error) {
	file, err := s.repo.GetKnowledgeBaseFile(ctx, knowledgeBaseID, fileID)
	if err != nil {
		return nil, mapFileRepositoryError(err)
	}
	if !isTextDocument(*file) {
		return nil, ErrDocumentNotEditable
	}
	text, err := s.readDocumentText(ctx, *file)
	if err != nil {
		return nil, err
	}
	if !utf8.ValidString(text) {
		return nil, ErrDocumentNotEditable
	}
	title := strings.TrimSuffix(file.FileName, filepath.Ext(file.FileName))
	content := text
	if heading, body, ok := strings.Cut(text, "\n\n"); ok && strings.HasPrefix(heading, "# ") && !strings.Contains(heading, "\n") {
		title, content = strings.TrimPrefix(heading, "# "), body
	}
	return &domainknowledgebase.TextDocument{FileID: file.FileID, Title: title, Content: content}, nil
}

func (s *Service) readDocumentText(ctx context.Context, file domainconversation.FileObject) (string, error) {
	opened, err := s.fileOpener.OpenFileContent(ctx, file.UserID, file.FileID)
	if err != nil {
		return "", err
	}
	defer opened.Reader.Close()
	content, err := io.ReadAll(opened.Reader)
	return string(content), err
}

func isTextDocument(file domainconversation.FileObject) bool {
	mediaType, _, _ := mime.ParseMediaType(file.MimeType)
	return mediaType == "text/markdown" || mediaType == "text/plain"
}

// markEditableDocuments 根据文本格式开放编辑入口，读取时再校验 UTF-8。
func markEditableDocuments(files []domainconversation.FileObject) {
	for i := range files {
		files[i].Editable = isTextDocument(files[i])
	}
}
