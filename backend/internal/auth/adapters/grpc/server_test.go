package grpc

import (
	"context"
	"crypto/ed25519"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"aivo/internal/auth/app"
	authv1 "aivo/internal/auth/v1"
	"aivo/pkg/tokenauth"

	"uuid"
)

func TestMint_ValidRequest(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	s := New(app.New(priv))

	userID, tenantID := uuid.New(), uuid.New()
	resp, err := s.Mint(context.Background(), &authv1.MintRequest{
		UserId:   userID.String(),
		TenantId: tenantID.String(),
		Roles:    []string{"owner"},
		AppId:    "admin",
	})
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if resp.GetToken() == "" {
		t.Fatal("expected a non-empty token")
	}

	pub := priv.Public().(ed25519.PublicKey)
	claims, err := tokenauth.Verify(pub, resp.GetToken())
	if err != nil {
		t.Fatalf("tokenauth.Verify: %v", err)
	}
	if claims.UserID != userID || claims.TenantID != tenantID {
		t.Fatal("verified claims do not match the minted request")
	}
}

func TestMint_RejectsMalformedRequests(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	s := New(app.New(priv))
	validUserID, validTenantID := uuid.New().String(), uuid.New().String()

	cases := []struct {
		name string
		req  *authv1.MintRequest
	}{
		{"invalid user_id", &authv1.MintRequest{UserId: "not-a-uuid", TenantId: validTenantID, Roles: []string{"owner"}, AppId: "admin"}},
		{"empty user_id", &authv1.MintRequest{UserId: "", TenantId: validTenantID, Roles: []string{"owner"}, AppId: "admin"}},
		{"invalid tenant_id", &authv1.MintRequest{UserId: validUserID, TenantId: "not-a-uuid", Roles: []string{"owner"}, AppId: "admin"}},
		{"empty tenant_id", &authv1.MintRequest{UserId: validUserID, TenantId: "", Roles: []string{"owner"}, AppId: "admin"}},
		{"missing roles", &authv1.MintRequest{UserId: validUserID, TenantId: validTenantID, AppId: "admin"}},
		{"unknown app_id", &authv1.MintRequest{UserId: validUserID, TenantId: validTenantID, Roles: []string{"owner"}, AppId: "not-a-surface"}},
		{"empty app_id", &authv1.MintRequest{UserId: validUserID, TenantId: validTenantID, Roles: []string{"owner"}}},
		{"empty request", &authv1.MintRequest{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := s.Mint(context.Background(), tc.req)
			if err == nil {
				t.Fatalf("Mint(%s) = %v, nil, want an error", tc.name, resp)
			}
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("Mint(%s) code = %v, want InvalidArgument", tc.name, status.Code(err))
			}
		})
	}
}

// TestMint_OverGRPC proves the adapter is wired correctly end-to-end
// (proto (de)serialization, service registration), not just callable as
// a plain Go method.
func TestMint_OverGRPC(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { lis.Close() })

	grpcServer := grpc.NewServer()
	authv1.RegisterAuthServiceServer(grpcServer, New(app.New(priv)))
	go grpcServer.Serve(lis)
	t.Cleanup(grpcServer.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	client := authv1.NewAuthServiceClient(conn)
	userID, tenantID := uuid.New(), uuid.New()
	resp, err := client.Mint(context.Background(), &authv1.MintRequest{
		UserId:   userID.String(),
		TenantId: tenantID.String(),
		Roles:    []string{"manager"},
		AppId:    "pos",
	})
	if err != nil {
		t.Fatalf("Mint over gRPC: %v", err)
	}
	if resp.GetToken() == "" {
		t.Fatal("expected a non-empty token")
	}
}
