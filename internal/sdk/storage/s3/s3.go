// Package s3 implements the storage.Backend interface using S3-compatible
// object storage (MinIO). Uploads are idempotent: objects already present
// at their key are skipped so that retrying a failed job does not
// re-upload files that already landed.
package s3

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/sdk/storage"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Backend stores generated SDK files in S3-compatible object storage.
type Backend struct {
	client *minio.Client
	bucket string
}

// New creates a new S3 Backend from the given config.
func New(cfg config.S3Config) (*Backend, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("s3 backend: %w", err)
	}
	return &Backend{client: client, bucket: cfg.Bucket}, nil
}

// Upload walks localDir and streams each file to S3 under keyPrefix.
// Files that already exist at their object key are skipped, making the
// operation idempotent - safe to retry after a partial failure.
//
// Object key pattern: keyPrefix + "/" + relative_path_within_localDir
func (b *Backend) Upload(ctx context.Context, keyPrefix string, localDir string) (string, error) {
	err := filepath.WalkDir(localDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, _ := filepath.Rel(localDir, path)
		objectKey := keyPrefix + "/" + rel

		// Idempotency: check whether the object already exists before uploading.
		_, statErr := b.client.StatObject(ctx, b.bucket, objectKey, minio.StatObjectOptions{})
		if statErr == nil {
			// Object already present - skip (idempotent retry).
			return nil
		}
		// Treat anything other than "not found" as an unexpected error.
		minioErr := minio.ToErrorResponse(statErr)
		if minioErr.StatusCode != 404 && minioErr.Code != "NoSuchKey" {
			return fmt.Errorf("s3 stat %s: %w", objectKey, statErr)
		}

		// Object does not exist; stream the file from disk.
		fi, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		_, err = b.client.PutObject(ctx, b.bucket, objectKey, f, fi.Size(),
			minio.PutObjectOptions{ContentType: "application/octet-stream"})
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("s3 upload %s: %w", objectKey, err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("s3://%s/%s", b.bucket, keyPrefix), nil
}

// GetFile streams a single object by its exact key. The caller must close the
// returned ReadCloser. Returns os.ErrNotExist if the key is not found.
func (b *Backend) GetFile(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	obj, err := b.client.GetObject(ctx, b.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("s3 get %s: %w", key, err)
	}
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		minioErr := minio.ToErrorResponse(err)
		if minioErr.StatusCode == 404 || minioErr.Code == "NoSuchKey" {
			return nil, 0, fmt.Errorf("s3 get %s: %w", key, io.ErrUnexpectedEOF)
		}
		return nil, 0, fmt.Errorf("s3 stat %s: %w", key, err)
	}
	return obj, info.Size, nil
}

// Download retrieves all objects under the given key prefix.
func (b *Backend) Download(ctx context.Context, key string) ([]*storage.File, error) {
	var files []*storage.File
	for obj := range b.client.ListObjects(ctx, b.bucket, minio.ListObjectsOptions{
		Prefix:    key + "/",
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("s3 list %s: %w", key, obj.Err)
		}
		o, err := b.client.GetObject(ctx, b.bucket, obj.Key, minio.GetObjectOptions{})
		if err != nil {
			return nil, fmt.Errorf("s3 get %s: %w", obj.Key, err)
		}
		data, err := io.ReadAll(o)
		_ = o.Close()
		if err != nil {
			return nil, fmt.Errorf("s3 read %s: %w", obj.Key, err)
		}
		relPath := obj.Key[len(key)+1:]
		files = append(files, &storage.File{Path: relPath, Content: data})
	}
	return files, nil
}
