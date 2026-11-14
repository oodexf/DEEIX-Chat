package conversation

import (
	"context"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/dberror"
)

// AuthorizeConversationProjectTool 每次执行以一次查询确认会话、项目和可写知识库归属。
func (r *Repo) AuthorizeConversationProjectTool(ctx context.Context, userID uint, conversationID uint, projectID uint, knowledgeBasePublicID string) error {
	query := r.db.WithContext(ctx).Table("chat_conversations AS c").Select("p.id").
		Joins("JOIN chat_conversation_projects AS p ON p.id = c.project_id").
		Where("c.id = ? AND c.user_id = ? AND c.project_id = ? AND c.deleted_at IS NULL AND p.user_id = ? AND p.deleted_at IS NULL", conversationID, userID, projectID, userID)
	if knowledgeBasePublicID != "" {
		query = query.Joins("JOIN chat_conversation_project_knowledge_bases AS pk ON pk.project_id = p.id").
			Joins("JOIN knowledge_bases AS kb ON kb.id = pk.knowledge_base_id").
			Where("kb.public_id = ? AND kb.scope = ? AND kb.owner_user_id = ?", knowledgeBasePublicID, "user", userID)
	}
	var project struct{ ID uint }
	return dberror.Translate(query.Take(&project).Error)
}
