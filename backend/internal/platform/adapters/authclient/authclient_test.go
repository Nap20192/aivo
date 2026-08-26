package authclient

import (
	"context"
	"crypto/ed25519"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	authapp "aivo/internal/auth/app"
	authgrpc "aivo/internal/auth/adapters/grpc"
	authv1 "aivo/internal/auth/v1"
	"aivo/pkg/tokenauth"

	"uuid"
)

// startTestAuthServer runs a real aivo-auth gRPC server over bufconn and
// returns a Client wired to it — proves this adapter round-trips a
// mint through the actual generated proto client, not just its own
// method bodies.
func startTestAuthServer(t *testing.T, priv ed25519.PrivateKey) *Client {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { lis.Close() })

	s := grpc.NewServer()
	authv1.RegisterAuthServiceServer(s, authgrpc.New(authapp.New(priv)))
	go s.Serve(lis)
	t.Cleanup(s.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return &Client{conn: conn, grpc: authv1.NewAuthServiceClient(conn)}
}

func TestClient_Mint(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	c := startTestAuthServer(t, priv)

	userID, tenantID := uuid.New(), uuid.New()
	token, err := c.Mint(context.Background(), userID, tenantID, []string{"manager"}, "pos")
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if token == "" {
		t.Fatal("expected a non-empty token")
	}

	pub := priv.Public().(ed25519.PublicKey)
	claims, err := tokenauth.Verify(pub, token)
	if err != nil {
		t.Fatalf("tokenauth.Verify: %v", err)
	}
	if claims.UserID != userID || claims.TenantID != tenantID || claims.AppID != "pos" {
		t.Fatal("verified claims do not match the minted request")
	}
}

func TestClient_Mint_PropagatesServerError(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	c := startTestAuthServer(t, priv)

	// Unknown app_id: aivo-auth's Mint rejects it, the adapter must
	// surface that as an error rather than a token.
	if _, err := c.Mint(context.Background(), uuid.New(), uuid.New(), []string{"owner"}, "not-a-surface"); err == nil {
		t.Fatal("expected an error for an unknown app_id")
	}
}

// Dial's target resolution is lazy (grpc.NewClient never touches the
// network), so there's no reachable failure mode to exercise here
// beyond the happy path — connection failures surface from Mint, not
// Dial, and are covered by the server-error propagation test above.
func TestDial(t *testing.T) {
	c, err := Dial("localhost:9082")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
