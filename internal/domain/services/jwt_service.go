package services

import (
	"clarity-ai/internal/domain/models"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTService interface {
	GenerateToken(userID int64, plan models.UserPlan, email string) (string, error)
	ValidateToken(tokenString string) (*TokenClaims, error)
}

type jwtService struct {
	secretKey string
	duration  time.Duration
}

func NewJWTService(secretKey string, duration time.Duration) JWTService {
	return &jwtService{
		secretKey: secretKey,
		duration:  duration,
	}
}

type JWTClaims struct {
	UserID int64           `json:"user_id"`
	Plan   models.UserPlan `json:"plan"`
	Email  string          `json:"email"`
	jwt.RegisteredClaims
}

func (j *jwtService) GenerateToken(userID int64, plan models.UserPlan, email string) (string, error) {
	claims := JWTClaims{
		UserID: userID,
		Plan:   plan,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(j.duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(j.secretKey))
}

func (j *jwtService) ValidateToken(tokenString string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(j.secretKey), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return &TokenClaims{
			UserID: claims.UserID,
			Plan:   claims.Plan,
			Email:  claims.Email,
		}, nil
	}

	return nil, fmt.Errorf("invalid token")
}
