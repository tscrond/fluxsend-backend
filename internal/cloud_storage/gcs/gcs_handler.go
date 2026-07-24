package gcs

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"go.uber.org/zap"

	"github.com/tscrond/fluxsend-backend/internal/cloud_storage/types"
	"github.com/tscrond/fluxsend-backend/internal/logger"
	"github.com/tscrond/fluxsend-backend/internal/mappings"
	"github.com/tscrond/fluxsend-backend/pkg"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type GCSBucketHandler struct {
	log                   *zap.SugaredLogger
	Client                *storage.Client
	ServiceAccountKeyPath string
	BaseBucketName        string
	GoogleProjectID       string
}

func NewGCSBucketHandler(log *zap.SugaredLogger, svcaccountPath, bucketName, projId string) (types.ObjectStorage, error) {
	if strings.TrimSpace(svcaccountPath) == "" {
		return nil, errors.New("GOOGLE_APPLICATION_CREDENTIALS is empty for STORAGE_PROVIDER=gcs")
	}

	var err error
	for i := 0; i < 5; i++ {
		_, err = os.Stat(svcaccountPath)
		if err == nil {
			break
		}
		log.Warnw("retrying to find credentials file", "path", svcaccountPath, "error", err)
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		log.Errorw("failed to find credentials file after retries", "path", svcaccountPath, "error", err)
		return nil, err
	}

	client, err := storage.NewClient(context.Background(), option.WithCredentialsFile(svcaccountPath))
	if err != nil {
		log.Errorw("error initializing GCS client", "error", err)
		return nil, err
	}

	return &GCSBucketHandler{
		log:                   log,
		Client:                client,
		ServiceAccountKeyPath: svcaccountPath,
		BaseBucketName:        bucketName,
		GoogleProjectID:       projId,
	}, nil
}

func (b *GCSBucketHandler) CreateMultipartUpload(ctx context.Context, bucket, uploadPath, contentType string) (*string, error) {
	return nil, nil
}

func (b *GCSBucketHandler) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) (*types.PutObjectResult, error) {
	writer := b.Client.Bucket(bucket).Object(key).NewWriter(ctx)
	writer.ContentType = contentType

	if _, err := io.Copy(writer, r); err != nil {
		logger.FromContext(ctx).Errorw("error uploading file", "error", err)
		return nil, fmt.Errorf("%w: %v", types.ErrStorageUnavailable, err)
	}
	if err := writer.Close(); err != nil {
		logger.FromContext(ctx).Errorw("error closing writer", "error", err)
		return nil, fmt.Errorf("%w: %v", types.ErrStorageUnavailable, err)
	}

	attrs, err := b.Client.Bucket(bucket).Object(key).Attrs(ctx)
	if err != nil {
		logger.FromContext(ctx).Errorw("error reading object attrs", "error", err)
		return nil, fmt.Errorf("%w: %v", types.ErrStorageUnavailable, err)
	}

	return &types.PutObjectResult{
		MD5:         hex.EncodeToString(attrs.MD5),
		Size:        attrs.Size,
		ContentType: attrs.ContentType,
	}, nil
}

func (b *GCSBucketHandler) BucketExists(ctx context.Context, fullBucketName string) (bool, error) {
	_, err := b.Client.Bucket(fullBucketName).Attrs(ctx)
	if err == storage.ErrBucketNotExist {
		logger.FromContext(ctx).Infow("bucket does not exist", "bucket", fullBucketName)
		return false, nil
	}
	return err == nil, err
}

func (b *GCSBucketHandler) checkObjExists(ctx context.Context, bucketName, objName string) (bool, error) {
	obj := b.Client.Bucket(bucketName).Object(objName)

	_, err := obj.Attrs(ctx)
	if err == storage.ErrObjectNotExist {
		return false, nil
	}
	if err != nil {
		logger.FromContext(ctx).Errorw("error checking object existence", "error", err)
		return false, err
	}

	return true, nil
}

func (b *GCSBucketHandler) CreateBucketIfNotExists(ctx context.Context, userId string) error {

	bucketName := pkg.GetUserBucketName(b.BaseBucketName, userId)

	exists, err := b.BucketExists(ctx, bucketName)
	if !exists {
		if err := b.CreateBucket(ctx, bucketName, b.GoogleProjectID); err != nil {
			logger.FromContext(ctx).Errorw("error creating storage bucket", "bucket", bucketName, "error", err)
			return err
		}
		return nil
	}
	if err != nil {
		logger.FromContext(ctx).Errorw("error checking for bucket", "bucket", bucketName, "error", err)
		return err
	}

	return nil
}

func (b *GCSBucketHandler) getBucketAttrs(ctx context.Context, bucketName string) (*mappings.BucketData, error) {
	bucketDataAttrs, err := b.Client.Bucket(bucketName).Attrs(ctx)
	if err != nil {
		return nil, err
	}

	if bucketDataAttrs.Labels != nil {
		// fmt.Printf("\n\n\nLabels:")
		for key, value := range bucketDataAttrs.Labels {
			fmt.Printf("\t%v = %v\n", key, value)
		}
	}

	return &mappings.BucketData{
		BucketName:   bucketDataAttrs.Name,
		StorageClass: bucketDataAttrs.StorageClass,
		TimeCreated:  bucketDataAttrs.Created,
		Labels:       bucketDataAttrs.Labels,
	}, nil
}

func (b *GCSBucketHandler) getObjectsAttrs(ctx context.Context, bucketName string) ([]mappings.ObjectMedatata, error) {
	var objects []mappings.ObjectMedatata
	it := b.Client.Bucket(bucketName).Objects(ctx, nil)
	for {
		objAttrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			logger.FromContext(ctx).Warnw("error iterating objects", "error", err)
			continue
		}
		// log.Printf("%+v\n", objAttrs)

		objects = append(objects, mappings.ObjectMedatata{
			Name:        objAttrs.Name,
			ContentType: objAttrs.ContentType,
			Created:     objAttrs.Created,
			Deleted:     objAttrs.Deleted,
			Updated:     objAttrs.Updated,
			MD5:         string(hex.EncodeToString(objAttrs.MD5)),
			Size:        objAttrs.Size,
			MediaLink:   objAttrs.MediaLink,
			Bucket:      objAttrs.Bucket,
		})
	}

	return objects, nil
}

func (b *GCSBucketHandler) getObjectsAttrsByObjName(ctx context.Context, bucketName, objName string) (*mappings.ObjectMedatata, error) {
	var selectedObj *mappings.ObjectMedatata
	objects, err := b.getObjectsAttrs(ctx, bucketName)
	if err != nil {
		logger.FromContext(ctx).Errorw("error getting objects attributes", "error", err)
		return nil, err
	}
	for _, o := range objects {
		if o.Name == objName {
			selectedObj = &o
		}
	}
	return selectedObj, nil
}

func (b *GCSBucketHandler) GetUserBucketData(ctx context.Context, id string) (any, error) {

	bucketName := pkg.GetUserBucketName(b.BaseBucketName, id)

	bucketData, err := b.getBucketAttrs(ctx, bucketName)
	if err != nil {
		logger.FromContext(ctx).Errorw("error getting bucket metadata", "bucket", bucketName, "error", err)
		return nil, err
	}

	objects, err := b.getObjectsAttrs(ctx, bucketName)
	if err != nil {
		logger.FromContext(ctx).Errorw("error getting objects metadata", "bucket", bucketName, "error", err)
		return nil, err
	}

	bucketData.Objects = objects

	return bucketData, nil
}

func (b *GCSBucketHandler) getUserBucketName(userInternalID string) string {
	return pkg.GetUserBucketName(b.BaseBucketName, userInternalID)
}

func (b *GCSBucketHandler) CreateBucket(ctx context.Context, fullBucketName, projectID string) error {
	bucket := b.Client.Bucket(fullBucketName)
	// userData, _ := ctx.Value(userdata.AuthorizedUserContextKey).(*userdata.AuthorizedUserInfo)

	err := bucket.Create(ctx, projectID, &storage.BucketAttrs{
		Location: "europe-west1",
		UniformBucketLevelAccess: storage.UniformBucketLevelAccess{
			Enabled: true,
		},
		PublicAccessPrevention: storage.PublicAccessPreventionEnforced,
	})
	if err != nil {
		return err
	}

	logger.FromContext(ctx).Infow("bucket created", "bucket", fullBucketName)
	return err
}

func (b *GCSBucketHandler) Close() error {
	if b.Client != nil {
		return b.Client.Close()
	}
	return nil
}

func (b *GCSBucketHandler) GenerateSignedURL(ctx context.Context, bucket, object string, expiresAt time.Time, contentDisposition string) (string, error) {

	email, pkey, err := pkg.LoadServiceAccount(b.ServiceAccountKeyPath)
	if err != nil {
		return "", fmt.Errorf("Bucket(%q) error reading svc account: %w", bucket, err)
	}

	opts := &storage.SignedURLOptions{
		Scheme:         storage.SigningSchemeV4,
		Method:         "GET",
		Expires:        expiresAt,
		GoogleAccessID: email,
		PrivateKey:     pkey,
	}
	if contentDisposition != "" {
		opts.QueryParameters = url.Values{
			"response-content-disposition": []string{contentDisposition},
		}
	}

	u, err := storage.SignedURL(bucket, object, opts)

	if err != nil {
		return "", fmt.Errorf("Bucket(%q).SignedURL: %w", bucket, err)
	}

	u = html.UnescapeString(u)

	// fmt.Println(u)

	return u, nil
}

func (b *GCSBucketHandler) GetBucketBaseName() string {
	return b.BaseBucketName
}

func (b *GCSBucketHandler) DeleteObjectsFromBucket(ctx context.Context, objects []string, bucket string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var o *storage.ObjectHandle
	for _, object := range objects {
		o = b.Client.Bucket(bucket).Object(object)
		attrs, err := o.Attrs(ctx)
		if err != nil {
			return fmt.Errorf("object.Attrs: %w", err)
		}
		o = o.If(storage.Conditions{GenerationMatch: attrs.Generation})

		if err := o.Delete(ctx); err != nil {
			return fmt.Errorf("Object(%q).Delete: %w", object, err)
		}

		logger.FromContext(ctx).Infow("object deleted", "bucket", o.BucketName(), "key", o.ObjectName())
	}

	return nil
}

func (b *GCSBucketHandler) DeleteObjectFromBucket(ctx context.Context, object, bucket string) error {

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	o := b.Client.Bucket(bucket).Object(object)

	// From GCP official docs: https://cloud.google.com/storage/docs/deleting-objects
	// Optional: set a generation-match precondition to avoid potential race
	// conditions and data corruptions. The request to delete the file is aborted
	// if the object's generation number does not match your precondition.
	attrs, err := o.Attrs(ctx)
	if err != nil {
		return fmt.Errorf("object.Attrs: %w", err)
	}
	o = o.If(storage.Conditions{GenerationMatch: attrs.Generation})

	if err := o.Delete(ctx); err != nil {
		return fmt.Errorf("Object(%q).Delete: %w", object, err)
	}

	logger.FromContext(ctx).Infow("object deleted", "bucket", o.BucketName(), "key", o.ObjectName())
	return nil
}

func (b *GCSBucketHandler) MoveObjectInBucket(ctx context.Context, source, destination, bucket string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	srcObj := b.Client.Bucket(bucket).Object(source)
	dstObj := b.Client.Bucket(bucket).Object(destination)

	if _, err := dstObj.CopierFrom(srcObj).Run(ctx); err != nil {
		return fmt.Errorf("copy %q -> %q failed: %w", source, destination, err)
	}

	if err := srcObj.Delete(ctx); err != nil {
		return fmt.Errorf("delete source %q after copy failed: %w", source, err)
	}

	return nil
}

func (b *GCSBucketHandler) getAllObjectNames(ctx context.Context, bucket string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	objectNames := []string{}

	it := b.Client.Bucket(bucket).Objects(ctx, nil)
	for {
		attrs, err := it.Next()

		if err == iterator.Done {
			break
		}
		if err != nil {
			return objectNames, fmt.Errorf("Bucket(%q).Objects: %w", bucket, err)
		}
		objectNames = append(objectNames, attrs.Name)
	}

	return objectNames, nil
}

func (b *GCSBucketHandler) DeleteBucket(ctx context.Context, bucket string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute*1)
	defer cancel()

	objectsInBucket, err := b.getAllObjectNames(ctx, bucket)
	if err != nil {
		logger.FromContext(ctx).Errorw("failed fetching bucket info", "bucket", bucket, "error", err)
	}

	gcsBucket := b.Client.Bucket(bucket)
	for _, o := range objectsInBucket {
		object := gcsBucket.Object(o)
		if err := object.Delete(ctx); err != nil {
			logger.FromContext(ctx).Errorw("failed deleting object", "object", o, "error", err)
		}
		logger.FromContext(ctx).Infow("deleted object", "object", o)
	}

	if err := gcsBucket.Delete(ctx); err != nil {
		logger.FromContext(ctx).Errorw("failed deleting bucket", "bucket", bucket, "error", err)
		return fmt.Errorf("failed_deleting_bucket")
	}

	logger.FromContext(ctx).Infow("deleted bucket", "bucket", bucket)
	return nil
}
