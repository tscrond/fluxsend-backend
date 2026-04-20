/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
)

var (
	dryRun    bool
	deleteOld bool
)

// s3Cmd represents the s3 command
var s3Cmd = &cobra.Command{
	Use:   "s3",
	Short: "Migrate S3 keys from google_id to UUID",
	Run: func(cmd *cobra.Command, args []string) {
		dbURL := os.Getenv("DATABASE_URL")
		bucketName := os.Getenv("S3_BUCKET_NAME")
		region := os.Getenv("AWS_REGION")

		if dbURL == "" || bucketName == "" || region == "" {
			log.Fatal("DATABASE_URL, S3_BUCKET_NAME, and AWS_REGION must be set")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		err := MigrateS3Keys(S3MigrationParams{
			DbURL:      dbURL,
			BucketName: bucketName,
			Region:     region,
			DryRun:     dryRun,
			DeleteOld:  deleteOld,
		}, ctx)

		if err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	s3Cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview migration without changes")
	s3Cmd.Flags().BoolVar(&deleteOld, "delete-old", false, "Delete old objects after copy")

	rootCmd.AddCommand(s3Cmd)
}

type S3MigrationParams struct {
	DbURL      string
	BucketName string
	Region     string
	DryRun     bool
	DeleteOld  bool
}

func MigrateS3Keys(migrationParams S3MigrationParams, ctx context.Context) error {

	// Connect to database
	db, err := sql.Open("postgres", migrationParams.DbURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// Get user mappings: google_id -> uuid
	rows, err := db.QueryContext(ctx, "SELECT google_id, id::text FROM users")
	if err != nil {
		log.Fatalf("failed to query users: %v", err)
	}
	defer rows.Close()

	type userMapping struct {
		GoogleID string
		UUID     string
	}
	var users []userMapping

	for rows.Next() {
		var u userMapping
		if err := rows.Scan(&u.GoogleID, &u.UUID); err != nil {
			log.Fatalf("failed to scan row: %v", err)
		}
		users = append(users, u)
	}

	if len(users) == 0 {
		log.Println("No users found, nothing to migrate.")
		return nil
	}

	log.Printf("Found %d users to migrate", len(users))

	// Initialize S3 client
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(migrationParams.Region))
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}
	client := s3.NewFromConfig(cfg)

	totalCopied := 0
	totalDeleted := 0

	for _, u := range users {
		if u.GoogleID == u.UUID {
			log.Printf("  [skip] User %s: google_id equals UUID (already migrated?)", u.GoogleID)
			continue
		}

		oldPrefix := u.GoogleID + "/"
		newPrefix := u.UUID + "/"

		log.Printf("  Migrating user: google_id=%s -> uuid=%s", u.GoogleID, u.UUID)

		// List all objects under the old prefix
		listOutput, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket: aws.String(migrationParams.BucketName),
			Prefix: aws.String(oldPrefix),
		})
		if err != nil {
			log.Printf("    [error] Failed to list objects for prefix %s: %v", oldPrefix, err)
			continue
		}

		if len(listOutput.Contents) == 0 {
			log.Printf("    [skip] No objects found under prefix %s", oldPrefix)
			continue
		}

		log.Printf("    Found %d objects to migrate", len(listOutput.Contents))

		var copiedKeys []string

		for _, obj := range listOutput.Contents {
			oldKey := aws.ToString(obj.Key)
			relativePath := strings.TrimPrefix(oldKey, oldPrefix)
			newKey := newPrefix + relativePath

			if migrationParams.DryRun {
				fmt.Printf("    [dry-run] Would copy: %s -> %s\n", oldKey, newKey)
				continue
			}

			// Copy object to new key — CopySource must be URL-encoded per segment
			encodedKey := encodeS3Key(oldKey)
			copySource := fmt.Sprintf("%s/%s", migrationParams.BucketName, encodedKey)
			_, err := client.CopyObject(ctx, &s3.CopyObjectInput{
				Bucket:     aws.String(migrationParams.BucketName),
				CopySource: aws.String(copySource),
				Key:        aws.String(newKey),
			})
			if err != nil {
				log.Printf("    [error] Failed to copy %s -> %s: %v", oldKey, newKey, err)
				continue
			}

			log.Printf("    [copied] %s -> %s", oldKey, newKey)
			copiedKeys = append(copiedKeys, oldKey)
			totalCopied++
		}

		// Delete old objects if requested
		if migrationParams.DeleteOld && !migrationParams.DryRun && len(copiedKeys) > 0 {
			var objectIds []s3types.ObjectIdentifier
			for _, key := range copiedKeys {
				objectIds = append(objectIds, s3types.ObjectIdentifier{
					Key: aws.String(key),
				})
			}

			_, err := client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(migrationParams.BucketName),
				Delete: &s3types.Delete{Objects: objectIds},
			})
			if err != nil {
				log.Printf("    [error] Failed to delete old objects: %v", err)
			} else {
				log.Printf("    [deleted] %d old objects", len(copiedKeys))
				totalDeleted += len(copiedKeys)
			}
		}
	}

	log.Printf("\nMigration complete: %d objects copied, %d old objects deleted", totalCopied, totalDeleted)
	if migrationParams.DryRun {
		log.Println("(dry-run mode — no changes were made)")
	}
	if !migrationParams.DeleteOld && totalCopied > 0 {
		log.Println("Old objects were NOT deleted. Run with --delete-old to remove them after verifying.")
	}

	return nil
}

// encodeS3Key URL-encodes each segment of an S3 key, preserving '/' separators.
func encodeS3Key(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}
