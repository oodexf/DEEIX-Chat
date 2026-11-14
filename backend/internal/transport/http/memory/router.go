package memory

import "github.com/gin-gonic/gin"

// RegisterRoutes 注册记忆域路由。
func (m *Module) RegisterRoutes(authRequired *gin.RouterGroup) {
	authRequired.GET("/conversation-projects/:id/memories", m.Handler.ListProjectMemories)
	authRequired.PUT("/conversation-projects/:id/memories", m.Handler.UpsertProjectMemory)
	authRequired.DELETE("/conversation-projects/:id/memories/:memoryID", m.Handler.DeleteProjectMemory)
	authRequired.GET("/memories/profile", m.Handler.ListUserMemories)
	authRequired.PUT("/memories/profile", m.Handler.UpsertUserMemory)
	authRequired.DELETE("/memories/profile/:memory_key", m.Handler.DeleteUserMemory)
}
