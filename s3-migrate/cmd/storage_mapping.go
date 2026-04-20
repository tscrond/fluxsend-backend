package cmd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"cloud.google.com/go/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
)

type fileMappingRow struct {
	UserID         string
	UserBucket     string
	LogicalName    string
	StorageMapping string
}

var (
	mappingDryRun    bool
	mappingDeleteOld bool
)

var storageMappingCmd = &cobra.Command{
	Use:   "storage-mapping",
	Short: "Migrate bucket object names from logical file names to storage_mapping UUID keys",
}

var storageMappingS3Cmd = &cobra.Command{
	Use:   "s3",
	Short: "Migrate S3 objects from <user_id>/<file_name> to <user_id>/<storage_mapping>",
	Run: func(cmd *cobra.Command, args []string) {
		dbURL := os.Getenv("DATABASE_URL")
		bucketName := os.Getenv("S3_BUCKET_NAME")
		region := os.Getenv("AWS_REGION")

		if dbURL == "" || bucketName == "" || region == "" {
			log.Fatal("DATABASE_URL, S3_BUCKET_NAME, and AWS_REGION must be set")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		if err := MigrateStorageMappingS3(ctx, dbURL, bucketName, region, mappingDryRun, mappingDeleteOld); err != nil {
			log.Fatal(err)
		}
	},
}

var storageMappingGCSCmd = &cobra.Command{
	Use:   "gcs",
	Short: "Migrate GCS objects from <file_name> to <storage_mapping> inside each user bucket",
	Run: func(cmd *cobra.Command, args []string) {
		dbURL := os.Getenv("DATABASE_URL")
		baseBucketName := os.Getenv("GCS_BUCKET_NAME")

		if dbURL == "" || baseBucketName == "" {
			log.Fatal("DATABASE_URL and GCS_BUCKET_NAME must be set")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		if err := MigrateStorageMappingGCS(ctx, dbURL, baseBucketName, mappingDryRun, mappingDeleteOld); err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	storageMappingS3Cmd.Flags().BoolVar(&mappingDryRun, "dry-run", false, "Preview migration without changes")
	storageMappingS3Cmd.Flags().BoolVar(&mappingDeleteOld, "delete-old", false, "Delete old objects after successful copy")

	storageMappingGCSCmd.Flags().BoolVar(&mappingDryRun, "dry-run", false, "Preview migration without changes")
	storageMappingGCSCmd.Flags().BoolVar(&mappingDeleteOld, "delete-old", false, "Delete old objects after successful copy")

	storageMappingCmd.AddCommand(storageMappingS3Cmd)
	storageMappingCmd.AddCommand(storageMappingGCSCmd)
	rootCmd.AddCommand(storageMappingCmd)
}

func loadFileMappings(ctx context.Context, db *sql.DB) ([]fileMappingRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			u.id::text AS user_id,
			COALESCE(u.user_bucket, '') AS user_bucket,
			f.file_name,
			f.storage_mapping::text
		FROM files f
		JOIN users u ON u.id = f.owner_id
		ORDER BY u.id, f.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mappings := make([]fileMappingRow, 0)
	for rows.Next() {
		var row fileMappingRow
		if err := rows.Scan(&row.UserID, &row.UserBucket, &row.LogicalName, &row.StorageMapping); err != nil {
			return nil, err
		}
		mappings = append(mappings, row)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return mappings, nil
}

func objectExistsS3(ctx context.Context, client *s3.Client, bucketName, key string) (bool, error) {
	_, err := client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}

	var notFound *s3types.NotFound
	if errors.As(err, &notFound) {
		return false, nil
	}

	return false, err
}

func MigrateStorageMappingS3(ctx context.Context, dbURL, bucketName, region string, dryRun, deleteOld bool) error {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to DB: %w", err)
	}
	defer db.Close()

	rows, err := loadFileMappings(ctx, db)
	if err != nil {
		return fmt.Errorf("failed to load file mappings: %w", err)
	}
	if len(rows) == 0 {
		log.Println("No files found in DB. Nothing to migrate.")
		return nil
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return fmt.Errorf("failed to init AWS config: %w", err)
	}
	client := s3.NewFromConfig(cfg)

	copied := 0
	deleted := 0
	skipped := 0

	for _, row := range rows {
		oldKey := row.UserID + "/" + row.LogicalName
		newKey := row.UserID + "/" + row.StorageMapping

		if oldKey == newKey {
			skipped++
			continue
		}

		if dryRun {
			log.Printf("[dry-run] S3 copy %s -> %s", oldKey, newKey)
			continue
		}

		exists, err := objectExistsS3(ctx, client, bucketName, newKey)
		if err != nil {
			log.Printf("[warn] failed to check destination key %s: %v", newKey, err)
			continue
		}
		if exists {
			log.Printf("[skip] destination already exists: %s", newKey)
			if deleteOld {
				_, delErr := client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucketName), Key: aws.String(oldKey)})
				if delErr == nil {
					deleted++
				}
			}
			skipped++
			continue
		}

		copySource := fmt.Sprintf("%s/%s", bucketName, encodeS3Key(oldKey))
		_, err = client.CopyObject(ctx, &s3.CopyObjectInput{
			Bucket:     aws.String(bucketName),
			CopySource: aws.String(copySource),
			Key:        aws.String(newKey),
		})
		if err != nil {
			log.Printf("[warn] failed to copy %s -> %s: %v", oldKey, newKey, err)
			continue
		}
		copied++

		if deleteOld {
			_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucketName), Key: aws.String(oldKey)})
			if err != nil {
				log.Printf("[warn] copied but failed deleting old key %s: %v", oldKey, err)
			} else {
				deleted++
			}
		}
	}

	log.Printf("S3 storage_mapping migration done: copied=%d deleted=%d skipped=%d dry_run=%t", copied, deleted, skipped, dryRun)
	return nil
}

func objectExistsGCS(ctx context.Context, client *storage.Client, bucketName, key string) (bool, error) {
	_, err := client.Bucket(bucketName).Object(key).Attrs(ctx)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, storage.ErrObjectNotExist) {
		return false, nil
	}
	return false, err
}

func MigrateStorageMappingGCS(ctx context.Context, dbURL, baseBucketName string, dryRun, deleteOld bool) error {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to DB: %w", err)
	}
	defer db.Close()

	rows, err := loadFileMappings(ctx, db)
	if err != nil {
		return fmt.Errorf("failed to load file mappings: %w", err)
	}
	if len(rows) == 0 {
		log.Println("No files found in DB. Nothing to migrate.")
		return nil
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to init GCS client: %w", err)
	}
	defer client.Close()

	copied := 0
	deleted := 0
	skipped := 0

	for _, row := range rows {
		bucketName := row.UserBucket
		if bucketName == "" {
			bucketName = baseBucketName + "-" + row.UserID
		}

		oldKey := row.LogicalName
		newKey := row.StorageMapping
		if oldKey == newKey {
			skipped++
			continue
		}

		if dryRun {
			log.Printf("[dry-run] GCS copy %s/%s -> %s/%s", bucketName, oldKey, bucketName, newKey)
			continue
		}

		destinationExists, err := objectExistsGCS(ctx, client, bucketName, newKey)
		if err != nil {
			log.Printf("[warn] failed checking destination object %s/%s: %v", bucketName, newKey, err)
			continue
		}
		if destinationExists {
			log.Printf("[skip] destination already exists: %s/%s", bucketName, newKey)
			if deleteOld {
				if delErr := client.Bucket(bucketName).Object(oldKey).Delete(ctx); delErr == nil {
					deleted++
				}
			}
			skipped++
			continue
		}

		sourceObj := client.Bucket(bucketName).Object(oldKey)
		if _, err := sourceObj.Attrs(ctx); err != nil {
			if errors.Is(err, storage.ErrObjectNotExist) {
				log.Printf("[skip] source object not found: %s/%s", bucketName, oldKey)
				skipped++
				continue
			}
			log.Printf("[warn] failed checking source object %s/%s: %v", bucketName, oldKey, err)
			continue
		}

		destinationObj := client.Bucket(bucketName).Object(newKey)
		if _, err := destinationObj.CopierFrom(sourceObj).Run(ctx); err != nil {
			log.Printf("[warn] failed to copy %s/%s -> %s/%s: %v", bucketName, oldKey, bucketName, newKey, err)
			continue
		}
		copied++

		if deleteOld {
			if err := sourceObj.Delete(ctx); err != nil {
				log.Printf("[warn] copied but failed deleting old object %s/%s: %v", bucketName, oldKey, err)
			} else {
				deleted++
			}
		}
	}

	log.Printf("GCS storage_mapping migration done: copied=%d deleted=%d skipped=%d dry_run=%t", copied, deleted, skipped, dryRun)
	return nil
}
