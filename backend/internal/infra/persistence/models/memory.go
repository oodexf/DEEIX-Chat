package models

// UserMemory 记录用户长期个性化记忆。
type UserMemory struct {
	BaseModel
	UserID             uint   `gorm:"not null;index:idx_user_memories_user_id;comment:用户ID"`
	MemoryKey          string `gorm:"size:128;not null;uniqueIndex:idx_user_memories_user_key;comment:记忆键"`
	Value              string `gorm:"type:text;not null;default:'';comment:记忆值"`
	Scope              string `gorm:"size:32;not null;default:'';index:idx_user_memories_scope;comment:记忆范围"`
	UpdatedBy          string `gorm:"size:32;not null;default:'';comment:更新来源"`
	EmbeddingSignature string `gorm:"size:64;not null;default:'';index:idx_user_memories_embedding_signature;comment:Embedding模型与维度签名"`
}

// TableName 指定表名。
func (UserMemory) TableName() string {
	return "user_memories"
}

// ProjectMemory 记录项目共享记忆，硬删除后可重新使用同一记忆键。
type ProjectMemory struct {
	ControlPlaneModel
	ProjectID uint   `gorm:"not null;uniqueIndex:idx_project_memories_project_key,priority:1;comment:项目ID"`
	MemoryKey string `gorm:"size:128;not null;uniqueIndex:idx_project_memories_project_key,priority:2;comment:记忆键"`
	Value     string `gorm:"type:text;not null;comment:记忆内容"`
	Source    string `gorm:"size:16;not null;comment:写入来源(user/assistant/auto)"`
}

// TableName 指定表名。
func (ProjectMemory) TableName() string { return "chat_conversation_project_memories" }
