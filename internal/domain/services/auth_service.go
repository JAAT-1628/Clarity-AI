package services

import (
	"clarity-ai/internal/domain/models"
	"clarity-ai/internal/domain/repositories"
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type AuthService interface {
	Register(ctx context.Context, req *RegisterRequest) (*models.User, error)
	Login(ctx context.Context, req *LoginRequest) (*AuthResponse, error)
	ValidateToken(tokenString string) (*TokenClaims, error)
}

type RegisterRequest struct {
	Username string `json:"username" validate:"required,min=3,max=50"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=6"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=6"`
}

type AuthResponse struct {
	User  *models.User `json:"user"`
	Token string       `json:"token"`
}

type TokenClaims struct {
	UserID int64           `json:"user_id"`
	Plan   models.UserPlan `json:"plan"`
	Email  string          `json:"email"`
}

type authService struct {
	userRepo   repositories.UserRepository
	jwtService JWTService
}

func NewAuthService(userRepo repositories.UserRepository, jwtService JWTService) AuthService {
	return &authService{
		userRepo:   userRepo,
		jwtService: jwtService,
	}
}

func (s *authService) Register(ctx context.Context, req *RegisterRequest) (*models.User, error) {
	extUser, err := s.userRepo.GetUserByEmail(ctx, req.Email)
	if err == nil && extUser != nil {
		return nil, fmt.Errorf("user with email %s already exists", req.Email)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := &models.User{
		Username:          req.Username,
		Email:             req.Email,
		Password:          string(hashedPassword),
		Plan:              models.PlanFree,
		APIUsageCount:     0,
		APIUsageResetDate: time.Now(),
	}

	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	user.Password = ""
	return user, nil
}

func (s *authService) Login(ctx context.Context, req *LoginRequest) (*AuthResponse, error) {
	user, err := s.userRepo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, fmt.Errorf("invalid password")
	}

	token, err := s.jwtService.GenerateToken(user.ID, user.Plan, user.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	user.Password = ""

	return &AuthResponse{
		User:  user,
		Token: token,
	}, nil
}

func (s *authService) ValidateToken(tokenString string) (*TokenClaims, error) {
	return s.jwtService.ValidateToken(tokenString)
}
