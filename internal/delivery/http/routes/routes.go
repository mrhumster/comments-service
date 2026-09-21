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
//   - public read: top-level comments per stream + replies, both gated on
//     published && non-private stream status (fail-closed 404/403/503) and
//     rate limited per client
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
	if err := r.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
		// Invalid config should not silently re-enable spoofable ClientIP;
		// trust none (direct peer only) and surface the misconfiguration.
		log.Printf("trusted proxies: %v", err)
		_ = r.SetTrustedProxies(nil)
	}
	r.Use(middleware.MetricsMiddleware())
	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.Server.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	}))

	h := handler.NewCommentsHandler(svc)

	// Public read, gated on stream commentability (published && non-private)
	// and rate limited per direct peer IP (trusted-proxy list is empty by
	// default, so a spoofed X-Forwarded-For does not grant a fresh budget).
	public := r.Group("", middleware.RateLimitPerMin(cfg.Server.ReadRateLimitPerMin))
	{
		public.GET("/streams/:streamId/comments", h.ListTop)
		public.GET("/comments/:id/replies", h.ListReplies)
	}

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
