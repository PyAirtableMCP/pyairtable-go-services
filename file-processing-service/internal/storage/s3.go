package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/pyairtable/go-services/file-processing-service/internal/models"
)

// S3Storage implements the storage interface using AWS S3 or S3-compatible storage
type S3Storage struct {
	client *s3.Client
	bucket string
	config *models.S3Config
}

// NewS3Storage creates a new S3 storage backend
func NewS3Storage(cfg *models.S3Config) (*S3Storage, error) {
	var awsConfig aws.Config
	var err error

	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		// Use provided credentials
		creds := credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			cfg.SessionToken,
		)
		
		awsConfig, err = config.LoadDefaultConfig(context.TODO(),
			config.WithCredentialsProvider(creds),
			config.WithRegion(cfg.Region),
		)
	} else {
		// Use default credential chain
		awsConfig, err = config.LoadDefaultConfig(context.TODO(),
			config.WithRegion(cfg.Region),
		)
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client options
	s3Options := []func(*s3.Options){}
	
	if cfg.Endpoint != "" {
		s3Options = append(s3Options, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		})
	}
	
	if cfg.ForcePathStyle {
		s3Options = append(s3Options, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsConfig, s3Options...)

	return &S3Storage{
		client: client,
		bucket: cfg.Bucket,
		config: cfg,
	}, nil
}

// StreamingUpload uploads a file using streaming
func (s *S3Storage) StreamingUpload(ctx context.Context, params *UploadParams) (*models.StorageInfo, error) {
	key := GenerateKey(params.FileID, params.Filename)
	
	if err := ValidateKey(key); err != nil {
		return nil, err
	}

	// Prepare metadata
	metadata := make(map[string]string)
	if params.Metadata != nil {
		for k, v := range params.Metadata {
			metadata[k] = v
		}
	}
	metadata["file-id"] = params.FileID
	metadata["original-filename"] = params.Filename

	// Upload object
	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        params.Reader,
		ContentType: aws.String(params.ContentType),
		Metadata:    metadata,
	}

	if params.Size > 0 {
		input.ContentLength = aws.Int64(params.Size)
	}

	result, err := s.client.PutObject(ctx, input)
	if err != nil {
		return nil, &StorageError{Op: "upload", Err: err}
	}

	// Create storage info
	storageInfo := &models.StorageInfo{
		Bucket:      s.bucket,
		Key:         key,
		Size:        params.Size,
		ContentType: params.ContentType,
	}

	if result.ETag != nil {
		storageInfo.URL = s.generatePublicURL(key)
	}

	return storageInfo, nil
}

// InitMultipartUpload initializes a multipart upload
func (s *S3Storage) InitMultipartUpload(ctx context.Context, params *MultipartUploadParams) (*UploadInfo, error) {
	key := GenerateKey(params.FileID, params.Filename)
	
	if err := ValidateKey(key); err != nil {
		return nil, err
	}

	// Prepare metadata
	metadata := make(map[string]string)
	if params.Metadata != nil {
		for k, v := range params.Metadata {
			metadata[k] = v
		}
	}
	metadata["file-id"] = params.FileID
	metadata["original-filename"] = params.Filename

	input := &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(params.ContentType),
		Metadata:    metadata,
	}

	result, err := s.client.CreateMultipartUpload(ctx, input)
	if err != nil {
		return nil, &StorageError{Op: "init_multipart", Err: err}
	}

	uploadInfo := &UploadInfo{
		UploadID:  *result.UploadId,
		Key:       key,
		ExpiresAt: time.Now().Add(24 * time.Hour), // S3 multipart uploads expire after 24 hours by default
	}

	return uploadInfo, nil
}

// UploadPart uploads a part in a multipart upload
func (s *S3Storage) UploadPart(ctx context.Context, params *UploadPartParams) (*PartInfo, error) {
	key := GenerateKey(params.FileID, "")
	
	input := &s3.UploadPartInput{
		Bucket:     aws.String(s.bucket),
		Key:        aws.String(key),
		UploadId:   aws.String(params.UploadID),
		PartNumber: aws.Int32(int32(params.PartNumber)),
		Body:       params.Reader,
	}

	if params.Size > 0 {
		input.ContentLength = aws.Int64(params.Size)
	}

	result, err := s.client.UploadPart(ctx, input)
	if err != nil {
		return nil, &StorageError{Op: "upload_part", Err: err}
	}

	partInfo := &PartInfo{
		PartNumber: params.PartNumber,
		ETag:       strings.Trim(*result.ETag, "\""),
		Size:       params.Size,
	}

	return partInfo, nil
}

// CompleteMultipartUpload completes a multipart upload
func (s *S3Storage) CompleteMultipartUpload(ctx context.Context, params *CompleteMultipartUploadParams) (*models.StorageInfo, error) {
	key := GenerateKey(params.FileID, "")
	
	// Convert parts to S3 format
	completedParts := make([]types.CompletedPart, len(params.Parts))
	var totalSize int64
	
	for i, part := range params.Parts {
		completedParts[i] = types.CompletedPart{
			PartNumber: aws.Int32(int32(part.PartNumber)),
			ETag:       aws.String(part.ETag),
		}
	}

	input := &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(params.UploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	}

	result, err := s.client.CompleteMultipartUpload(ctx, input)
	if err != nil {
		return nil, &StorageError{Op: "complete_multipart", Err: err}
	}

	// Get object info to retrieve size and content type
	headInput := &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}
	
	headResult, err := s.client.HeadObject(ctx, headInput)
	if err == nil {
		totalSize = *headResult.ContentLength
	}

	storageInfo := &models.StorageInfo{
		Bucket: s.bucket,
		Key:    key,
		Size:   totalSize,
		URL:    s.generatePublicURL(key),
	}

	if result.ETag != nil {
		// Set content type if available
		if headResult != nil && headResult.ContentType != nil {
			storageInfo.ContentType = *headResult.ContentType
		}
	}

	return storageInfo, nil
}

// AbortMultipartUpload aborts a multipart upload
func (s *S3Storage) AbortMultipartUpload(ctx context.Context, fileID, uploadID string) error {
	key := GenerateKey(fileID, "")
	
	input := &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(s.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	}

	_, err := s.client.AbortMultipartUpload(ctx, input)
	if err != nil {
		return &StorageError{Op: "abort_multipart", Err: err}
	}

	return nil
}

// Download downloads a file
func (s *S3Storage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}

	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	result, err := s.client.GetObject(ctx, input)
	if err != nil {
		return nil, &StorageError{Op: "download", Err: err}
	}

	return result.Body, nil
}

// GetDownloadURL generates a pre-signed download URL
func (s *S3Storage) GetDownloadURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}

	presignClient := s3.NewPresignClient(s.client)
	
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	result, err := presignClient.PresignGetObject(ctx, input, func(opts *s3.PresignOptions) {
		opts.Expires = expiry
	})
	if err != nil {
		return "", &StorageError{Op: "presign_url", Err: err}
	}

	return result.URL, nil
}

// Delete deletes a file
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}

	input := &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	_, err := s.client.DeleteObject(ctx, input)
	if err != nil {
		return &StorageError{Op: "delete", Err: err}
	}

	return nil
}

// Exists checks if a file exists
func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	if err := ValidateKey(key); err != nil {
		return false, err
	}

	input := &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	_, err := s.client.HeadObject(ctx, input)
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}
		return false, &StorageError{Op: "exists", Err: err}
	}

	return true, nil
}

// GetFileInfo gets information about a file
func (s *S3Storage) GetFileInfo(ctx context.Context, key string) (*FileInfo, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}

	input := &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}

	result, err := s.client.HeadObject(ctx, input)
	if err != nil {
		return nil, &StorageError{Op: "get_file_info", Err: err}
	}

	fileInfo := &FileInfo{
		Key:         key,
		Size:        *result.ContentLength,
		ContentType: aws.ToString(result.ContentType),
		ETag:        strings.Trim(aws.ToString(result.ETag), "\""),
		Metadata:    result.Metadata,
	}

	if result.LastModified != nil {
		fileInfo.LastModified = *result.LastModified
	}

	return fileInfo, nil
}

// ListFiles lists files with optional prefix
func (s *S3Storage) ListFiles(ctx context.Context, prefix string, limit int) ([]*FileInfo, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
	}

	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}

	if limit > 0 {
		input.MaxKeys = aws.Int32(int32(limit))
	}

	result, err := s.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, &StorageError{Op: "list_files", Err: err}
	}

	files := make([]*FileInfo, len(result.Contents))
	for i, obj := range result.Contents {
		files[i] = &FileInfo{
			Key:          *obj.Key,
			Size:         *obj.Size,
			ETag:         strings.Trim(*obj.ETag, "\""),
			LastModified: *obj.LastModified,
		}
	}

	return files, nil
}

// Copy copies a file from one key to another
func (s *S3Storage) Copy(ctx context.Context, sourceKey, destKey string) error {
	if err := ValidateKey(sourceKey); err != nil {
		return err
	}
	if err := ValidateKey(destKey); err != nil {
		return err
	}

	copySource := fmt.Sprintf("%s/%s", s.bucket, sourceKey)
	
	input := &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		Key:        aws.String(destKey),
		CopySource: aws.String(url.QueryEscape(copySource)),
	}

	_, err := s.client.CopyObject(ctx, input)
	if err != nil {
		return &StorageError{Op: "copy", Err: err}
	}

	return nil
}

// Move moves a file from one key to another
func (s *S3Storage) Move(ctx context.Context, sourceKey, destKey string) error {
	// Copy first
	if err := s.Copy(ctx, sourceKey, destKey); err != nil {
		return err
	}

	// Then delete source
	if err := s.Delete(ctx, sourceKey); err != nil {
		// If delete fails, try to delete the destination to avoid duplicates
		s.Delete(ctx, destKey)
		return err
	}

	return nil
}

// GenerateThumbnail generates a thumbnail for supported file types
func (s *S3Storage) GenerateThumbnail(ctx context.Context, sourceKey, thumbnailKey string, width, height int) error {
	// This is a placeholder implementation
	// In a real implementation, you would:
	// 1. Download the source file
	// 2. Generate thumbnail using image processing library
	// 3. Upload the thumbnail
	return fmt.Errorf("thumbnail generation not implemented for S3 storage")
}

// Health checks the health of the storage backend
func (s *S3Storage) Health(ctx context.Context) error {
	input := &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	}

	_, err := s.client.HeadBucket(ctx, input)
	if err != nil {
		return &StorageError{Op: "health", Err: err}
	}

	return nil
}

// Helper methods

func (s *S3Storage) generatePublicURL(key string) string {
	if s.config.Endpoint != "" {
		// Custom endpoint (like MinIO)
		scheme := "https"
		if s.config.DisableSSL {
			scheme = "http"
		}
		
		if s.config.ForcePathStyle {
			return fmt.Sprintf("%s://%s/%s/%s", scheme, s.config.Endpoint, s.bucket, key)
		}
		return fmt.Sprintf("%s://%s.%s/%s", scheme, s.bucket, s.config.Endpoint, key)
	}
	
	// AWS S3
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.config.Region, key)
}