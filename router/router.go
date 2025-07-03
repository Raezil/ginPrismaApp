package router

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"db"
	. "middlewares"
	. "services"
)

// SetupRouter initializes Gin engine with all routes and rate limiting.
func SetupRouter(database *db.PrismaClient) *gin.Engine {
	r := gin.Default()

	r.Use(LoggingMiddleware())
	// Create rate limiters
	generalLimiter := NewRateLimiter(rate.Every(time.Second), 10) // 10 requests per second
	authLimiter := NewRateLimiter(rate.Every(time.Minute), 5)     // 5 requests per minute for auth

	// Start cleanup goroutine for expired limiters
	go generalLimiter.CleanupExpiredLimiters()
	go authLimiter.CleanupExpiredLimiters()

	// Apply general rate limiting to all routes
	r.Use(RateLimitMiddleware(generalLimiter))
	streaming := NewStreaming()
	// Public routes
	pub := r.Group("/api")
	{
		// Apply stricter rate limiting to authentication endpoints
		authRoutes := pub.Group("/")
		authRoutes.Use(StrictRateLimitMiddleware(authLimiter))
		{
			authRoutes.POST("/register", func(c *gin.Context) {
				var req struct {
					Username string `json:"username" binding:"required"`
					Password string `json:"password" binding:"required"`
					Email    string `json:"email" binding:"required,email"`
					Age      int    `json:"age" binding:"required,min=0"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}

				hash, err := HashPassword(req.Password)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "could not secure password"})
					return
				}

				database.User.CreateOne(
					db.User.Name.Set(req.Username),
					db.User.Password.Set(hash),
					db.User.Email.Set(req.Email),
					db.User.Age.Set(req.Age),
				).Exec(c.Request.Context())

				token, err := GenerateToken(req.Email)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"status": "registration successful", "token": token})
			})

			authRoutes.POST("/login", func(c *gin.Context) {
				var creds struct {
					Email    string `json:"email" binding:"required"`
					Password string `json:"password" binding:"required"`
				}
				if err := c.ShouldBindJSON(&creds); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}

				user, err := database.User.FindUnique(
					db.User.Email.Equals(creds.Email),
				).Exec(c.Request.Context())
				if err != nil || !CheckPassword(user.Password, creds.Password) {
					c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
					return
				}

				token, err := GenerateToken(user.Email)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"token": token})
			})
		}
	}

	// Protected routes
	prot := r.Group("/api")
	prot.Use(JwtMiddleware())
	{
		prot.GET("/profile", func(c *gin.Context) {
			email := c.GetString("email")
			c.JSON(http.StatusOK, gin.H{"message": "Welcome, " + email})
		})

		prot.GET("/video", func(c *gin.Context) {
			streaming.Stream(c.Writer, c.Request)
		})

		prot.POST("/video/upload", func(c *gin.Context) {
			streaming.UploadVideo(c)
		})
	}

	return r
}
