package user

import (
	pb "clarity-ai/api/proto/user"
	"clarity-ai/internal/domain/models"
	"clarity-ai/internal/domain/repositories"
	"clarity-ai/internal/domain/services"
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type UserServiceHandler struct {
	pb.UnimplementedUserServiceServer
	authService services.AuthService
	userRepo    repositories.UserRepository
}

func NewUserServiceHandler(authService services.AuthService, userRepo repositories.UserRepository) *UserServiceHandler {
	return &UserServiceHandler{
		authService: authService,
		userRepo:    userRepo,
	}
}

func (h *UserServiceHandler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if req.Username == "" || req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "username, email, and password are required")
	}

	registerReq := &services.RegisterRequest{
		Username: req.Username,
		Email:    req.Email,
		Password: req.Password,
	}

	user, err := h.authService.Register(ctx, registerReq)
	if err != nil {
		if contains(err.Error(), "already exists") {
			return nil, status.Error(codes.AlreadyExists, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	loginReq := &services.LoginRequest{
		Email:    req.Email,
		Password: req.Password,
	}

	authResponse, err := h.authService.Login(ctx, loginReq)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to generate token")
	}

	return &pb.RegisterResponse{
		User:  convertUserToProto(user),
		Token: authResponse.Token,
	}, nil
}

func (h *UserServiceHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	loginReq := &services.LoginRequest{
		Email:    req.Email,
		Password: req.Password,
	}

	authResponse, err := h.authService.Login(ctx, loginReq)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	return &pb.LoginResponse{
		User:  convertUserToProto(authResponse.User),
		Token: authResponse.Token,
	}, nil
}

func (h *UserServiceHandler) GetProfile(ctx context.Context, req *pb.GetProfileRequest) (*pb.GetProfileResponse, error) {
	if req.Id <= 0 {
		return nil, status.Error(codes.InvalidArgument, "valid user ID is required")
	}

	user, err := h.userRepo.GetUserID(ctx, req.Id)
	if err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	return &pb.GetProfileResponse{
		User: convertUserToProto(user),
	}, nil
}

func (h *UserServiceHandler) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	if req.Token == "" {
		return nil, status.Error(codes.InvalidArgument, "token is required")
	}

	claims, err := h.authService.ValidateToken(req.Token)
	if err != nil {
		return &pb.ValidateTokenResponse{
			Valid:  false,
			Claims: nil,
		}, nil
	}

	protoClaims := &pb.TokenClaims{
		UserId: claims.UserID,
		Email:  claims.Email,
		Plan:   convertPlanToProto(claims.Plan),
	}

	return &pb.ValidateTokenResponse{
		Valid:  true,
		Claims: protoClaims,
	}, nil
}

// Helper functions
func convertUserToProto(user *models.User) *pb.User {
	return &pb.User{
		Id:                user.ID,
		Username:          user.Username,
		Email:             user.Email,
		Plan:              convertPlanToProto(user.Plan),
		ApiUsageCount:     user.APIUsageCount,
		ApiUsageResetDate: timestamppb.New(user.APIUsageResetDate),
		CreatedAt:         timestamppb.New(user.CreatedAt),
		UpdatedAt:         timestamppb.New(user.UpdatedAt),
	}
}

func convertPlanToProto(plan models.UserPlan) pb.UserPlan {
	switch plan {
	case "free":
		return pb.UserPlan_free
	case "premium":
		return pb.UserPlan_premium
	case "pro":
		return pb.UserPlan_pro
	default:
		return pb.UserPlan_free
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr ||
		len(s) > len(substr) && s[:len(substr)] == substr ||
		len(s) > len(substr) && s[len(s)/2-len(substr)/2:len(s)/2+len(substr)/2] == substr
}
