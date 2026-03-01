package routes

import (
	"github.com/ThanadolU/hospital-middleware/internal/handler"
	"github.com/gin-gonic/gin"
)

func AuthRoute(r *gin.RouterGroup, authHandler *handler.AuthHandler) {
	authGroup := r.Group("/auth")

	authGroup.POST("/register", authHandler.Register)
	authGroup.POST("/login", authHandler.Login)
}