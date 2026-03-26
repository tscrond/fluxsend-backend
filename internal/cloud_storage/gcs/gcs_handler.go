package gcs

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/storage"

	"github.com/tscrond/dropper/internal/cloud_storage/types"
	"github.com/tscrond/dropper/internal/filedata"
	"github.com/tscrond/dropper/internal/mappings"
	"github.com/tscrond/dropper/internal/repo"
	"github.com/tscrond/dropper/internal/repo/sqlc"
	"github.com/tscrond/dropper/internal/userdata"
	"github.com/tscrond/dropper/pkg"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// TODO create named buckets <bucket_name>-<user_id> + restrict access by ID and token verification
type GCSBucketHandler struct {
	repository            *repo.Repository
	Client                *storage.Client
	ServiceAccountKeyPath string
	BaseBucketName        string
	GoogleProjectID       string
}

func NewGCSBucketHandler(svcaccountPath, bucketName, projId string, repository *repo.Repository) (types.ObjectStorage, error) {

	var err error
	for i := 0; i < 5; i++ {
		_, err = os.Stat(svcaccountPath)
		if err == nil {
			break
		}
		log.Printf("Retrying to find credentials file (%s): %v", svcaccountPath, err)
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		log.Printf("Failed to find credentials file after retries: %v\n", err)
		return nil, err
	}

	client, err := storage.NewClient(context.Background(), option.WithCredentialsFile(svcaccountPath))
	if err != nil {
		log.Println("Error initializing client:", err)
		return nil, err
	}

	return &GCSBucketHandler{
		repository:            repository,
		Client:                client,
		ServiceAccountKeyPath: svcaccountPath,
		BaseBucketName:        bucketName,
		GoogleProjectID:       projId,
	}, nil
}

// Handler that processes a single file per request
func (b *GCSBucketHandler) SendFileToBucket(ctx context.Context, data *filedata.FileData) error {

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

	// userBucketName := pkg.GetUserBucketName(b.BaseBucketName, authUserData.Id)
	userBucketName, err := b.repository.Queries.GetUserBucketById(ctx, authUserData.Id)
	if err != nil {
		log.Println(err)
		return err
	}

	// try to get users bucket name, if not existing - retrieve the ID from authorized context and update the database
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

	// write new object to the bucket
	writer := b.Client.Bucket(newUserBucketName).Object(fileName).NewWriter(ctx)
	contentType := data.RequestHeaders.Header.Get("Content-Type")
	if contentType == "" {
		// Fallback to detection if header missing
		buffer := make([]byte, 512)
		_, err := data.MultipartFile.Read(buffer)
		if err != nil && err != io.EOF {
			log.Println("failed to read file for content type detection:", err)
			return err
		}
		data.MultipartFile.Seek(0, io.SeekStart) // rewind for actual upload
		contentType = http.DetectContentType(buffer)
	}
	writer.ContentType = contentType
	log.Println(contentType)
	if _, err := io.Copy(writer, data.MultipartFile); err != nil {
		log.Println("error uploading file: ", err)
		return fmt.Errorf("%w: %v", types.ErrStorageUnavailable, err)
	}
	if err := writer.Close(); err != nil {
		log.Println("error closing writer:", err)
		return fmt.Errorf("%w: %v", types.ErrStorageUnavailable, err)
	}

	newlyCreatedObj := b.Client.Bucket(newUserBucketName).Object(fileName)

	objAttrs, err := newlyCreatedObj.Attrs(ctx)
	if err != nil {
		log.Println("err reading obj attrs: ", err)
		return fmt.Errorf("%w: %v", types.ErrStorageUnavailable, err)
	}

	// temporary fix
	randInt := rand.Int63()

	privateDownloadToken, err := pkg.GenerateSecureTokenFromID(randInt)
	if err != nil {
		log.Println("err generating token: ", err)
		return err
	}
	insertArgs := sqlc.InsertFileParams{
		OwnerGoogleID:        sql.NullString{Valid: true, String: authUserData.Id},
		FileName:             fileName,
		FileType:             sql.NullString{Valid: true, String: objAttrs.ContentType},
		Size:                 sql.NullInt64{Valid: true, Int64: objAttrs.Size},
		Md5Checksum:          string(hex.EncodeToString(objAttrs.MD5)),
		PrivateDownloadToken: sql.NullString{Valid: true, String: privateDownloadToken},
	}

	// ensure the object data is saved to DB if it does not exist
	file, err := b.repository.Queries.InsertFile(ctx, insertArgs)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("file already exists: %s\n", err)
			return types.ErrFileAlreadyExists
		} else {
			log.Println("error inserting file to DB, removing the object from the bucket: ", err)
			if err := newlyCreatedObj.Delete(ctx); err != nil {
				log.Printf("error Object(%v).Delete: %v\n", newlyCreatedObj, err)
				return types.ErrUploadFailed
			}
			return types.ErrUploadFailed
		}
	}
	log.Printf("file %s uploaded successfully (checksum: %v)", fileName, file.Md5Checksum)
	return nil
}

func (b *GCSBucketHandler) BucketExists(ctx context.Context, fullBucketName string) (bool, error) {
	_, err := b.Client.Bucket(fullBucketName).Attrs(ctx)
	if err == storage.ErrBucketNotExist {
		log.Println("bucket does not exist")
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
		log.Printf("error checking object existence: %v", err)
		return false, err
	}

	return true, nil
}

func (b *GCSBucketHandler) CreateBucketIfNotExists(ctx context.Context, userId string) error {

	bucketName := pkg.GetUserBucketName(b.BaseBucketName, userId)

	exists, err := b.BucketExists(ctx, bucketName)
	if !exists {
		if err := b.CreateBucket(ctx, bucketName, b.GoogleProjectID); err != nil {
			log.Println("error creating storage bucket: ", err)
			return err
		}
		return nil
	}
	if err != nil {
		log.Println("error checking for bucket: ", err)
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
			log.Println(err)
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
		log.Println("error getting objects attributes", err)
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
		log.Println("error getting bucket metadata: ", err)
		return nil, err
	}

	objects, err := b.getObjectsAttrs(ctx, bucketName)
	if err != nil {
		log.Println("error getting objects metadata: ", err)
		return nil, err
	}

	bucketData.Objects = objects

	return bucketData, nil
}

func (b *GCSBucketHandler) GetUserBucketName(ctx context.Context) (string, error) {
	authorizedUserData := ctx.Value(userdata.AuthorizedUserContextKey)

	authUserData, ok := authorizedUserData.(*userdata.AuthorizedUserInfo)
	if !ok {
		log.Println("cannot read authorized user data")
		return "", errors.New("cannot read authorized user data")
	}

	bucketName := pkg.GetUserBucketName(b.BaseBucketName, authUserData.Id)

	return bucketName, nil
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

	log.Printf("bucket %s created successfully", fullBucketName)
	return err
}

func (b *GCSBucketHandler) Close() error {
	if b.Client != nil {
		return b.Client.Close()
	}
	return nil
}

func (b *GCSBucketHandler) GenerateSignedURL(ctx context.Context, bucket, object string, expiresAt time.Time) (string, error) {

	email, pkey, err := pkg.LoadServiceAccount(b.ServiceAccountKeyPath)
	if err != nil {
		return "", fmt.Errorf("Bucket(%q) error reading svc account: %w", bucket, err)
	}

	u, err := storage.SignedURL(bucket, object, &storage.SignedURLOptions{
		Scheme:         storage.SigningSchemeV4,
		Method:         "GET",
		Expires:        expiresAt,
		GoogleAccessID: email,
		PrivateKey:     pkey,
		// Style:          storage.VirtualHostedStyle(),
	})

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

	log.Printf("object deleted successfully: (%s,%s)", o.BucketName(), o.ObjectName())
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
		log.Println("failed_fetching_bucket_info, err: ", err)
	}

	gcsBucket := b.Client.Bucket(bucket)
	for _, o := range objectsInBucket {
		object := gcsBucket.Object(o)
		if err := object.Delete(ctx); err != nil {
			log.Printf("failed deleting object %s, err: %s\n", o, err)
		}
		log.Printf("deleted object %s", o)
	}

	if err := gcsBucket.Delete(ctx); err != nil {
		log.Println(err)
		return fmt.Errorf("failed_deleting_bucket")
	}

	log.Printf("deleted bucket: %s\n", bucket)
	return nil
}
