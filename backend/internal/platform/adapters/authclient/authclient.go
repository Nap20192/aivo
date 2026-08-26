// Package authclient is platform's gRPC client for aivo-auth's Mint RPC
// — it implements platformports.TokenMinter. Connects without transport
// credentials (plaintext gRPC): both processes are assumed to sit on
// the same trusted internal network, the same trust boundary the shared
// Postgres DSN already crosses; TLS between internal services is out of
// scope for this pilot (see design.md's Risks section).
package authclient

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	authv1 "aivo/internal/auth/v1"

	"uuid"
)

// Client mints tokens by calling aivo-auth over gRPC.
type Client struct {
	conn *grpc.ClientConn
	grpc authv1.AuthServiceClient
}

// Dial connects to aivo-auth at addr (host:port). Connection setup is
// lazy (grpc.NewClient does not block on the network) — a Mint call
// against an unreachable addr fails at call time, not here.
func Dial(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("authclient: dial %s: %w", addr, err)
	}
	return &Client{conn: conn, grpc: authv1.NewAuthServiceClient(conn)}, nil
}

// Close releases the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Mint implements platformports.TokenMinter.
func (c *Client) Mint(ctx context.Context, userID, tenantID uuid.UUID, roles []string, appID string) (string, error) {
	resp, err := c.grpc.Mint(ctx, &authv1.MintRequest{
		UserId:   userID.String(),
		TenantId: tenantID.String(),
		Roles:    roles,
		AppId:    appID,
	})
	if err != nil {
		return "", err
	}
	return resp.GetToken(), nil
}
