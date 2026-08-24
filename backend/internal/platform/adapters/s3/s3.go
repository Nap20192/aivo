// Package s3 implements ports.ImageStore against MinIO/S3 via the
// minio-go client. The bucket is provisioned by docker-compose
// (minio-init) as publicly readable, so Put returns a plain public URL.
package s3

import (
	"context"
	"fmt"
	"io"
	"strings"

	"aivo/internal/platform/ports"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"uuid"
)

type ImageStore struct {
	client    *minio.Client
	bucket    string
	publicURL string // origin browsers use to load images, e.g. http://localhost:9000
}

var _ ports.ImageStore = (*ImageStore)(nil)

// New connects to the S3 endpoint. endpoint is host:port (no scheme);
// publicURL is the browser-facing origin for the same storage.
func New(endpoint, accessKey, secretKey, bucket, publicURL string, useSSL bool) (*ImageStore, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("s3: connect: %w", err)
	}
	return &ImageStore{client: client, bucket: bucket, publicURL: strings.TrimRight(publicURL, "/")}, nil
}

func (s *ImageStore) Put(ctx context.Context, restaurantID uuid.UUID, filename, contentType string, r io.Reader, size int64) (string, error) {
	// Key: restaurant-scoped prefix + random name; the original filename
	// only contributes its extension (never trusted as a path).
	ext := ""
	if i := strings.LastIndex(filename, "."); i >= 0 && len(filename)-i <= 8 && isAlnum(filename[i+1:]) {
		ext = strings.ToLower(filename[i:])
	}
	key := fmt.Sprintf("%s/%s%s", restaurantID, uuid.New(), ext)

	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", fmt.Errorf("s3: put: %w", err)
	}
	// key is uuid/uuid.ext — URL-safe by construction, no escaping needed.
	return s.publicURL + "/" + s.bucket + "/" + key, nil
}

func isAlnum(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
