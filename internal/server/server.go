package server

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/helthtech/core-users/internal/model"
	"github.com/helthtech/core-users/internal/service"
	pb "github.com/helthtech/core-users/pkg/proto/users"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type UserServer struct {
	pb.UnimplementedUserServiceServer
	authSvc *service.AuthService
	userSvc *service.UserService
}

func NewUserServer(authSvc *service.AuthService, userSvc *service.UserService) *UserServer {
	return &UserServer{authSvc: authSvc, userSvc: userSvc}
}

func (s *UserServer) PrepareAuth(ctx context.Context, _ *pb.PrepareAuthRequest) (*pb.PrepareAuthResponse, error) {
	key, err := s.authSvc.PrepareAuth(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "prepare auth: %v", err)
	}
	return &pb.PrepareAuthResponse{Key: key}, nil
}

func (s *UserServer) ResolveAuthKey(ctx context.Context, req *pb.ResolveAuthKeyRequest) (*pb.ResolveAuthKeyResponse, error) {
	valid, err := s.authSvc.ResolveAuthKey(ctx, req.Key)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "resolve auth key: %v", err)
	}
	if !valid {
		return &pb.ResolveAuthKeyResponse{Valid: false}, nil
	}

	pu, err := s.authSvc.CreateProvisionalUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create provisional user: %v", err)
	}
	return &pb.ResolveAuthKeyResponse{Valid: true, ProvisionalUserId: pu.ID.String()}, nil
}

func (s *UserServer) CreateProvisionalUser(ctx context.Context, _ *pb.CreateProvisionalUserRequest) (*pb.CreateProvisionalUserResponse, error) {
	pu, err := s.authSvc.CreateProvisionalUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create provisional user: %v", err)
	}
	return &pb.CreateProvisionalUserResponse{Id: pu.ID.String()}, nil
}

func (s *UserServer) VerifyPhone(ctx context.Context, req *pb.VerifyPhoneRequest) (*pb.VerifyPhoneResponse, error) {
	provisionalID := uuid.Nil
	if req.ProvisionalUserId != "" {
		var err error
		provisionalID, err = uuid.Parse(req.ProvisionalUserId)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid provisional_user_id")
		}
	}

	userID, token, isNew, err := s.authSvc.VerifyPhone(ctx, req.PhoneE164, provisionalID, req.AuthKey)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "verify phone: %v", err)
	}
	return &pb.VerifyPhoneResponse{
		UserId:    userID.String(),
		Token:     token,
		IsNewUser: isNew,
	}, nil
}

func (s *UserServer) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.UserResponse, error) {
	uid, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user_id")
	}
	u, err := s.userSvc.GetUser(ctx, uid)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "user not found")
	}
	return modelUserToProto(u), nil
}

func (s *UserServer) UpdateUser(ctx context.Context, req *pb.UpdateUserRequest) (*pb.UserResponse, error) {
	uid, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user_id")
	}
	updates := make(map[string]any)
	if req.DisplayName != nil {
		updates["display_name"] = *req.DisplayName
	}
	if req.Locale != nil {
		updates["locale"] = *req.Locale
	}
	if req.Timezone != nil {
		updates["timezone"] = *req.Timezone
	}
	if req.BirthDate != nil {
		updates["birth_date"] = *req.BirthDate
	}
	if req.Sex != nil {
		updates["sex"] = *req.Sex
	}
	if req.OnboardingCompleted != nil {
		updates["onboarding_completed"] = *req.OnboardingCompleted
	}
	u, err := s.userSvc.UpdateUser(ctx, uid, updates)
	if err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("update user: %v", err))
	}
	return modelUserToProto(u), nil
}

func (s *UserServer) ValidateToken(_ context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	valid, uid, utype, err := s.userSvc.ValidateToken(req.Token)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "validate token: %v", err)
	}
	return &pb.ValidateTokenResponse{Valid: valid, UserId: uid, UserType: utype}, nil
}

func (s *UserServer) CheckProvisionalOrphans(_ context.Context, _ *pb.CheckProvisionalOrphansRequest) (*pb.CheckProvisionalOrphansResponse, error) {
	return &pb.CheckProvisionalOrphansResponse{IsOrphan: true}, nil
}

func (s *UserServer) DeleteProvisionalUser(ctx context.Context, req *pb.DeleteProvisionalUserRequest) (*pb.DeleteProvisionalUserResponse, error) {
	uid, err := uuid.Parse(req.ProvisionalUserId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid id")
	}
	if err = s.authSvc.DeleteProvisionalUser(ctx, uid); err != nil {
		return nil, status.Errorf(codes.Internal, "delete: %v", err)
	}
	return &pb.DeleteProvisionalUserResponse{}, nil
}

func modelUserToProto(u *model.User) *pb.UserResponse {
	resp := &pb.UserResponse{
		Id:          u.ID.String(),
		PhoneE164:   u.PhoneE164,
		CreatedAt:   timestamppb.New(u.CreatedAt),
		UpdatedAt:   timestamppb.New(u.UpdatedAt),
		Locale:      u.Locale,
		Timezone:    u.Timezone,
		DisplayName: u.DisplayName,
		BirthDate:   u.BirthDate,
		Sex:         u.Sex,
		IsAdmin:     u.IsAdmin,
	}
	if u.PhoneVerifiedAt != nil {
		resp.PhoneVerifiedAt = timestamppb.New(*u.PhoneVerifiedAt)
	}
	if u.OnboardingCompletedAt != nil {
		t := timestamppb.New(*u.OnboardingCompletedAt)
		resp.OnboardingCompletedAt = t
	}
	return resp
}
