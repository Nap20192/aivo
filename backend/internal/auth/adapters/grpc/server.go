// Package grpc adapts internal/auth/app's Mint logic onto the generated
// AuthService gRPC server interface: proto <-> Go type conversion and
// error-code mapping only, no business logic.
package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"aivo/internal/auth/app"
	authv1 "aivo/internal/auth/v1"

	"uuid"
)

// Server implements authv1.AuthServiceServer.
type Server struct {
	authv1.UnimplementedAuthServiceServer
	App *app.App
}

func New(a *app.App) *Server {
	return &Server{App: a}
}

// Mint is the only method AuthService exposes — the proto message has
// no credential field, so there is no request shape to reject beyond
// what field validation below already covers.
func (s *Server) Mint(_ context.Context, req *authv1.MintRequest) (*authv1.MintResponse, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "user_id: invalid uuid")
	}
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "tenant_id: invalid uuid")
	}

	token, err := s.App.Mint(app.MintParams{
		UserID:   userID,
		TenantID: tenantID,
		Roles:    req.GetRoles(),
		AppID:    app.AppID(req.GetAppId()),
	})
	if err != nil {
		if errors.Is(err, app.ErrInvalid) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, "mint failed")
	}
	return &authv1.MintResponse{Token: token}, nil
}
