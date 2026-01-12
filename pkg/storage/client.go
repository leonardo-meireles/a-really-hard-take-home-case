package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/fly-io/162719/pkg/errors"
)

// Client provides S3 storage operations
type Client struct {
	s3Client *s3.Client
	bucket   string
}

// NewClient creates a new S3 client for anonymous access
func NewClient(ctx context.Context, bucket, region string) (*Client, error) {
	slog.Info("s3_client_init", "bucket", bucket, "region", region)

	// Load AWS config with anonymous credentials
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(aws.AnonymousCredentials{}),
	)
	if err != nil {
		slog.Error("aws_config_load_failed", "error", err)
		return nil, errors.Wrap(err, "failed to load AWS config")
	}

	// Create S3 client
	s3Client := s3.NewFromConfig(cfg)

	slog.Info("s3_client_created", "bucket", bucket)

	return &Client{
		s3Client: s3Client,
		bucket:   bucket,
	}, nil
}

// DownloadResult contains download metadata
type DownloadResult struct {
	LocalPath string
	SHA256    string
	Size      int64
}

// Download downloads an object from S3 and computes SHA256
func (c *Client) Download(ctx context.Context, s3Key, localPath string) (*DownloadResult, error) {
	slog.Info("s3_download_start", "bucket", c.bucket, "s3_key", s3Key)

	// Get object from S3
	result, err := c.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		slog.Error("s3_get_object_failed", "s3_key", s3Key, "error", err)
		return nil, errors.Wrap(err, "failed to get object from S3")
	}
	defer result.Body.Close()

	// Create local file
	f, err := os.Create(localPath)
	if err != nil {
		slog.Error("local_file_creation_failed", "path", localPath, "error", err)
		return nil, errors.Wrap(err, "failed to create local file")
	}
	defer f.Close()

	// Copy data and compute SHA256
	hash := sha256.New()
	writer := io.MultiWriter(f, hash)

	size, err := io.Copy(writer, result.Body)
	if err != nil {
		slog.Error("s3_download_failed", "s3_key", s3Key, "error", err)
		return nil, errors.Wrap(err, "failed to download file")
	}

	// Compute checksum
	checksum := hex.EncodeToString(hash.Sum(nil))

	slog.Info("s3_download_complete",
		"s3_key", s3Key,
		"size_mb", size/1024/1024,
		"local_path", localPath,
		"sha256", checksum[:16]+"...",
	)

	return &DownloadResult{
		LocalPath: localPath,
		SHA256:    checksum,
		Size:      size,
	}, nil
}

// ListObjects lists all objects in the bucket with a given prefix
func (c *Client) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	slog.Info("s3_list_start", "bucket", c.bucket, "prefix", prefix)

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
		Prefix: aws.String(prefix),
	}

	var keys []string
	paginator := s3.NewListObjectsV2Paginator(c.s3Client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			slog.Error("s3_list_failed", "prefix", prefix, "error", err)
			return nil, errors.Wrap(err, "failed to list objects")
		}

		for _, obj := range page.Contents {
			if obj.Key != nil {
				keys = append(keys, *obj.Key)
			}
		}
	}

	slog.Info("s3_list_complete", "prefix", prefix, "object_count", len(keys))

	return keys, nil
}

// Exists checks if an object exists in S3
func (c *Client) Exists(ctx context.Context, s3Key string) (bool, error) {
	_, err := c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(s3Key),
	})

	if err != nil {
		// Check if it's a NotFound error
		if err.Error() == "NotFound" {
			slog.Info("s3_object_not_found", "s3_key", s3Key)
			return false, nil
		}
		slog.Error("s3_head_object_failed", "s3_key", s3Key, "error", err)
		return false, errors.Wrap(err, "failed to check object existence")
	}

	slog.Info("s3_object_exists", "s3_key", s3Key)
	return true, nil
}
