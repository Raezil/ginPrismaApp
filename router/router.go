package router

import (
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"

	"db"
)

// hashPassword hashes a plaintext password.
func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// checkPassword compares a bcrypt-hash with a plaintext pwd.
func checkPassword(hashed, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password))
	return err == nil
}

// loadSecret loads JWT secret from env or fallback.
func loadSecret() []byte {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte(s)
	}
	return []byte("supersecretkey123")
}

// Claims represents JWT payload.
type Claims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

// generateToken issues a new JWT.
func generateToken(email string, secret []byte) (string, error) {
	claims := &Claims{
		Email: email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "myapp",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

// jwtMiddleware validates incoming JWTs.
func jwtMiddleware(secret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if len(auth) < 7 || auth[:7] != "Bearer " {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or malformed token"})
			return
		}
		tokenStr := auth[7:]

		tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return secret, nil
		})
		if err != nil || !tok.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		if claims, ok := tok.Claims.(*Claims); ok {
			c.Set("email", claims.Email)
		}
		c.Next()
	}
}

// SetupRouter initializes Gin engine with all routes.
func SetupRouter(database *db.PrismaClient) *gin.Engine {
	secret := loadSecret()
	r := gin.Default()

	// Public routes
	pub := r.Group("/api")
	{
		pub.POST("/register", func(c *gin.Context) {
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

			hash, err := hashPassword(req.Password)
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

			token, err := generateToken(req.Email, secret)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "registration successful", "token": token})
		})

		pub.POST("/login", func(c *gin.Context) {
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
			if err != nil || !checkPassword(user.Password, creds.Password) {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
				return
			}

			token, err := generateToken(user.Email, secret)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"token": token})
		})
	}

	// Protected routes
	prot := r.Group("/api")
	prot.Use(jwtMiddleware(secret))
	{
		prot.GET("/profile", func(c *gin.Context) {
			email := c.GetString("email")
			c.JSON(http.StatusOK, gin.H{"message": "Welcome, " + email})
		})
		// add more
	}

	return r
}
