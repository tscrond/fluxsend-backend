package s3

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/tscrond/dropper/internal/cloud_storage/types"
	"github.com/tscrond/dropper/internal/filedata"
	"github.com/tscrond/dropper/internal/mappings"
	"github.com/tscrond/dropper/internal/repo"
	"github.com/tscrond/dropper/internal/repo/sqlc"
	"github.com/tscrond/dropper/internal/userdata"
	"github.com/tscrond/dropper/pkg"
)

type S3BucketHandler struct {
	repository     *repo.Repository
	Client         *s3.Client
	PresignClient  *s3.PresignClient
	BaseBucketName string
	Region         string
}

func NewS3BucketHandler(bucketName, region string, repository *repo.Repository) (types.ObjectStorage, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
	if err != nil {
		log.Println("Error loading AWS config:", err)
		return nil, err
	}

	client := s3.NewFromConfig(cfg)
	presignClient := s3.NewPresignClient(client)

	return &S3BucketHandler{
		repository:     repository,
		Client:         client,
		PresignClient:  presignClient,
		BaseBucketName: bucketName,
		Region:         region,
	}, nil
}

// s3Key builds the S3 object key with the user prefix.
func s3Key(userId, fileName string) string {
	return userId + "/" + fileName
}

// extractUserIdFromBucket parses the userId from "<baseName>-<userId>" format.
func (b *S3BucketHandler) extractUserIdFromBucket(bucket string) string {
	return pkg.ExtractUserIdFromBucketName(b.BaseBucketName, bucket)
}

func (b *S3BucketHandler) SendFileToBucket(ctx context.Context, data *filedata.FileData) error {
	authorizedUserData := ctx.Value(userdata.AuthorizedUserContextKey)
	authUserData, ok := authorizedUserData.(*userdata.AuthorizedUserInfo)
	if !ok {
		log.Println("cannot read authorized user data")
		return types.ErrUploadFailed
	}

	if data == nil {
		log.Println("Data for bucket operation is empty")
		return types.ErrUploadFailed
	}

	fileName := data.RequestHeaders.Filename

	// Prepend folder to filename if provided
	if data.Folder != "" {
		fileName = data.Folder + "/" + fileName
	}

	_, err := b.repository.Queries.GetFileByOwnerAndName(ctx, sqlc.GetFileByOwnerAndNameParams{
		OwnerGoogleID: sql.NullString{Valid: true, String: authUserData.Id},
		FileName:      fileName,
	})
	if err == nil {
		return types.ErrFileAlreadyExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		log.Println("error checking existing file:", err)
		return types.ErrUploadFailed
	}

	// get or set user bucket name in DB (same pattern as GCS driver)
	userBucketName, err := b.repository.Queries.GetUserBucketById(ctx, authUserData.Id)
	if err != nil {
		log.Println(err)
		return err
	}

	newUserBucketName := userBucketName.String
	if !userBucketName.Valid || userBucketName.String == "" {
		retrievedBucketName, err := b.GetUserBucketName(ctx)
		if err != nil {
			log.Println("Cannot find users bucket!", err)
			return err
		}

		if err := b.repository.Queries.UpdateUserBucketNameById(ctx, sqlc.UpdateUserBucketNameByIdParams{
			UserBucket: sql.NullString{String: retrievedBucketName, Valid: true},
			GoogleID:   authUserData.Id,
		}); err != nil {
			log.Println(err)
			return err
		}
		newUserBucketName = retrievedBucketName
	}

	// detect content type
	contentType := data.RequestHeaders.Header.Get("Content-Type")
	if contentType == "" {
		buffer := make([]byte, 512)
		_, err := data.MultipartFile.Read(buffer)
		if err != nil && err != io.EOF {
			log.Println("failed to read file for content type detection:", err)
			return err
		}
		data.MultipartFile.Seek(0, io.SeekStart)
		contentType = http.DetectContentType(buffer)
	}
	log.Println(contentType)

	// compute the userId from the stored bucket name to build the S3 key
	userId := b.extractUserIdFromBucket(newUserBucketName)
	objectKey := s3Key(userId, fileName)

	// compute MD5 while reading
	hasher := md5.New()
	teeReader := io.TeeReader(data.MultipartFile, hasher)

	// upload to S3
	putOutput, err := b.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(b.BaseBucketName),
		Key:           aws.String(objectKey),
		Body:          teeReader,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(data.RequestHeaders.Size),
	})
	if err != nil {
		log.Println("error uploading file: ", err)
		return fmt.Errorf("%w: %v", types.ErrStorageUnavailable, err)
	}

	md5Hash := hex.EncodeToString(hasher.Sum(nil))

	// get size from HeadObject
	headOutput, err := b.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.BaseBucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		log.Println("err reading obj attrs: ", err)
		return fmt.Errorf("%w: %v", types.ErrStorageUnavailable, err)
	}

	objSize := *headOutput.ContentLength
	_ = putOutput

	randInt := rand.Int63()
	privateDownloadToken, err := pkg.GenerateSecureTokenFromID(randInt)
	if err != nil {
		log.Println("err generating token: ", err)
		return err
	}

	insertArgs := sqlc.InsertFileParams{
		OwnerGoogleID:        sql.NullString{Valid: true, String: authUserData.Id},
		FileName:             fileName,
		FileType:             sql.NullString{Valid: true, String: contentType},
		Size:                 sql.NullInt64{Valid: true, Int64: objSize},
		Md5Checksum:          md5Hash,
		PrivateDownloadToken: sql.NullString{Valid: true, String: privateDownloadToken},
	}

	file, err := b.repository.Queries.InsertFile(ctx, insertArgs)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("file already exists: %s\n", err)
			return types.ErrFileAlreadyExists
		} else {
			log.Println("error inserting file to DB, removing the object from the bucket: ", err)
			if _, delErr := b.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(b.BaseBucketName),
				Key:    aws.String(objectKey),
			}); delErr != nil {
				log.Printf("error deleting object %s: %v\n", objectKey, delErr)
				return types.ErrUploadFailed
			}
			return types.ErrUploadFailed
		}
	}
	log.Printf("file %s uploaded successfully (checksum: %v)", fileName, file.Md5Checksum)
	return nil
}

func (b *S3BucketHandler) BucketExists(ctx context.Context, fullBucketName string) (bool, error) {
	_, err := b.Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(b.BaseBucketName),
	})
	if err != nil {
		var notFound *s3types.NotFound
		if errors.As(err, &notFound) {
			log.Println("bucket does not exist")
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (b *S3BucketHandler) CreateBucketIfNotExists(ctx context.Context, userId string) error {
	exists, err := b.BucketExists(ctx, b.BaseBucketName)
	if err != nil {
		log.Println("error checking for bucket: ", err)
		return err
	}
	if !exists {
		_, err := b.Client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(b.BaseBucketName),
			CreateBucketConfiguration: &s3types.CreateBucketConfiguration{
				LocationConstraint: s3types.BucketLocationConstraint(b.Region),
			},
		})
		if err != nil {
			log.Println("error creating storage bucket: ", err)
			return err
		}
		log.Printf("bucket %s created successfully", b.BaseBucketName)
	}
	return nil
}

func (b *S3BucketHandler) GetUserBucketData(ctx context.Context, id string) (any, error) {
	prefix := id + "/"

	output, err := b.Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.BaseBucketName),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		log.Println("error listing objects: ", err)
		return nil, err
	}

	var objects []mappings.ObjectMedatata
	for _, obj := range output.Contents {
		key := aws.ToString(obj.Key)
		// strip the userId prefix for the display name
		name := strings.TrimPrefix(key, prefix)
		if name == "" {
			continue
		}

		// get detailed attrs per object
		head, err := b.Client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(b.BaseBucketName),
			Key:    obj.Key,
		})

		contentType := ""
		if err == nil && head.ContentType != nil {
			contentType = *head.ContentType
		}

		etag := ""
		if obj.ETag != nil {
			etag = strings.Trim(*obj.ETag, "\"")
		}

		objects = append(objects, mappings.ObjectMedatata{
			Name:        name,
			ContentType: contentType,
			Created:     safeTime(obj.LastModified),
			Updated:     safeTime(obj.LastModified),
			MD5:         etag,
			Size:        derefInt64(obj.Size),
			Bucket:      b.BaseBucketName,
		})
	}

	bucketData := &mappings.BucketData{
		BucketName:   pkg.GetUserBucketName(b.BaseBucketName, id),
		StorageClass: "STANDARD",
		TimeCreated:  time.Time{},
		Labels:       nil,
		Objects:      objects,
	}

	return bucketData, nil
}

func (b *S3BucketHandler) GetUserBucketName(ctx context.Context) (string, error) {
	authorizedUserData := ctx.Value(userdata.AuthorizedUserContextKey)
	authUserData, ok := authorizedUserData.(*userdata.AuthorizedUserInfo)
	if !ok {
		log.Println("cannot read authorized user data")
		return "", errors.New("cannot read authorized user data")
	}

	// return the same "<base>-<userId>" format for DB compatibility
	bucketName := pkg.GetUserBucketName(b.BaseBucketName, authUserData.Id)
	return bucketName, nil
}

func (b *S3BucketHandler) GetBucketBaseName() string {
	return b.BaseBucketName
}

func (b *S3BucketHandler) GenerateSignedURL(ctx context.Context, bucket, object string, expiresAt time.Time) (string, error) {
	// bucket may be "<base>-<userId>" from DB; extract userId to build the real S3 key
	userId := b.extractUserIdFromBucket(bucket)
	objectKey := object
	if userId != "" {
		objectKey = s3Key(userId, object)
	}

	presignResult, err := b.PresignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.BaseBucketName),
		Key:    aws.String(objectKey),
	}, s3.WithPresignExpires(time.Until(expiresAt)))
	if err != nil {
		return "", fmt.Errorf("Bucket(%q).PresignGetObject: %w", b.BaseBucketName, err)
	}

	return presignResult.URL, nil
}

func (b *S3BucketHandler) DeleteObjectFromBucket(ctx context.Context, object, bucket string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// bucket is "<base>-<userId>"; extract userId to build the S3 key
	userId := b.extractUserIdFromBucket(bucket)
	objectKey := object
	if userId != "" {
		objectKey = s3Key(userId, object)
	}

	_, err := b.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.BaseBucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return fmt.Errorf("Object(%q).Delete: %w", objectKey, err)
	}

	log.Printf("object deleted successfully: (%s,%s)", b.BaseBucketName, objectKey)
	return nil
}

func (b *S3BucketHandler) MoveObjectInBucket(ctx context.Context, source, destination, bucket string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	userId := b.extractUserIdFromBucket(bucket)
	sourceKey := source
	destinationKey := destination
	if userId != "" {
		sourceKey = s3Key(userId, source)
		destinationKey = s3Key(userId, destination)
	}

	copySource := b.BaseBucketName + "/" + sourceKey
	_, err := b.Client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(b.BaseBucketName),
		CopySource: aws.String(copySource),
		Key:        aws.String(destinationKey),
	})
	if err != nil {
		return fmt.Errorf("copy %q -> %q failed: %w", sourceKey, destinationKey, err)
	}

	_, err = b.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.BaseBucketName),
		Key:    aws.String(sourceKey),
	})
	if err != nil {
		return fmt.Errorf("delete source %q after copy failed: %w", sourceKey, err)
	}

	return nil
}

func (b *S3BucketHandler) DeleteBucket(ctx context.Context, bucket string) error {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	// bucket is "<base>-<userId>"; delete all objects under the user's prefix
	userId := b.extractUserIdFromBucket(bucket)
	if userId == "" {
		return fmt.Errorf("cannot extract userId from bucket name: %s", bucket)
	}

	prefix := userId + "/"
	output, err := b.Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.BaseBucketName),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		log.Println("failed_fetching_bucket_info, err: ", err)
		return err
	}

	if len(output.Contents) > 0 {
		var objectIds []s3types.ObjectIdentifier
		for _, obj := range output.Contents {
			objectIds = append(objectIds, s3types.ObjectIdentifier{
				Key: obj.Key,
			})
			log.Printf("deleting object %s", aws.ToString(obj.Key))
		}

		_, err = b.Client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(b.BaseBucketName),
			Delete: &s3types.Delete{Objects: objectIds},
		})
		if err != nil {
			log.Println("error batch deleting objects: ", err)
			return fmt.Errorf("failed_deleting_user_objects")
		}
	}

	log.Printf("deleted all objects for user prefix: %s", prefix)
	return nil
}

func (b *S3BucketHandler) Close() error {
	return nil
}

func safeTime(t *time.Time) time.Time {
	if t != nil {
		return *t
	}
	return time.Time{}
}

func derefInt64(p *int64) int64 {
	if p != nil {
		return *p
	}
	return 0
}
