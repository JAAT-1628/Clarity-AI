package middleware

import (
	pb "clarity-ai/api/proto/user"
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func JWTAuthMiddleware(userserviceAddr string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isPublicRoute(c.Request.URL.Path) {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Authorization required",
				"message": "Please provide a valid bearer tokne in the authorization header",
			})
			c.Abort()
			return
		}

		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Invalid authorization header format",
				"message": "Use: Authorization: Bearer <your-jwt-token>",
			})
			c.Abort()
			return
		}

		token := tokenParts[1]

		conn, err := grpc.Dial(userserviceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Authentication service unavailable"})
			c.Abort()
			return
		}
		defer conn.Close()

		client := pb.NewUserServiceClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := client.ValidateToken(ctx, &pb.ValidateTokenRequest{
			Token: token,
		})

		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Token validation failed",
				"message": "Please login again to get a new token",
			})
			c.Abort()
			return
		}

		if !resp.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Invalid or expired token",
				"message": "Please login again to get a new token",
			})
			c.Abort()
			return
		}

		c.Set("user_id", resp.Claims.UserId)
		c.Set("user_email", resp.Claims.Email)
		c.Set("user_plan", resp.Claims.Plan)

		if strings.Contains(c.Request.URL.Path, "/user/profile") {
			if idParam := c.Query("id"); idParam != "" {
				requestedID, err := strconv.ParseInt(idParam, 10, 64)
				if err != nil || requestedID != resp.Claims.UserId {
					c.JSON(http.StatusForbidden, gin.H{
						"error":   "Access denied",
						"message": "You can only access your own profile",
					})
					c.Abort()
					return
				}
			}
		}
		c.Next()
	}
}

func isPublicRoute(path string) bool {
	publicRoutes := []string{
		"/health",
		"/api/auth/register",
		"/api/auth/login",
		"/api/validate-token",
	}

	for _, route := range publicRoutes {
		if strings.HasPrefix(path, route) {
			return true
		}
	}
	return false
}
