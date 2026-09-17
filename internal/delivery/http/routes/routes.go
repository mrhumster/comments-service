package routes

import (
	"log"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/mrhumster/comments-service/config"
	"github.com/mrhumster/comments-service/internal/delivery/http/handler"
	"github.com/mrhumster/comments-service/internal/delivery/http/middleware"
	"github.com/mrhumster/comments-service/internal/service"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gorm.io/gorm"
)

// SetupRoutes builds the REST API:
//   - public read: top-level comments per stream + replies (list access
//     mirrors the public catalog; private/unlisted rely on URL knowledge)
//   - authenticated write: create/update/delete with verified-email gate
//   - health and metrics
func SetupRoutes(db *gorm.DB, cfg *config.Config, svc service.CommentsService, tokens *service.TokenService) *gin.Engine {
	if cfg.Server.Mode == "test" {
		gin.SetMode(gin.TestMode)
	} else if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.MetricsMiddleware())
	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.Server.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	}))

	h := handler.NewCommentsHandler(svc)

	// Public read. Stream membership is not checked here (there is no
	// stream-service call); this mirrors GetStream access semantics.
	r.GET("/streams/:streamId/comments", h.ListTop)
	r.GET("/comments/:id/replies", h.ListReplies)

	authed := r.Group("", middleware.AuthMiddleware(tokens), middleware.RateLimitPerMin(cfg.Server.WriteRateLimitPerMin))
	{
		authed.POST("/streams/:streamId/comments", h.Create)
		authed.PATCH("/comments/:id", h.Update)
		authed.DELETE("/comments/:id", h.Delete)
	}

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	r.GET("/health", func(c *gin.Context) {
		if sqlDB, err := db.DB(); err == nil {
			if err := sqlDB.Ping(); err != nil {
				log.Println("comments PG error: ", err.Error())
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "error": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "up"})
	})

	return r
}
