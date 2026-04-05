package mappings

import (
	"database/sql"

	"github.com/tscrond/fluxsend-backend/internal/repo/sqlc"
	"github.com/tscrond/fluxsend-backend/pkg"
)

func MapBucketDataToDBFormat(ownerGoogleID string, bucketData *BucketData) ([]sqlc.File, error) {
	var sqlFiles []sqlc.File
	for _, obj := range bucketData.Objects {
		privateDownloadToken, err := pkg.GenerateSecureTokenFromIDStr(ownerGoogleID)
		if err != nil {
			return nil, err
		}

		newFile := sqlc.File{
			OwnerGoogleID:        sql.NullString{Valid: true, String: ownerGoogleID},
			FileName:             obj.Name,
			FileType:             sql.NullString{Valid: true, String: obj.ContentType},
			Size:                 sql.NullInt64{Valid: true, Int64: obj.Size},
			Md5Checksum:          obj.MD5,
			PrivateDownloadToken: sql.NullString{Valid: true, String: privateDownloadToken},
		}

		sqlFiles = append(sqlFiles, newFile)
	}

	return sqlFiles, nil
}

func FindMissingFilesFromDB(s1 []sqlc.File, s2 []sqlc.File) []sqlc.File {
	var diff []sqlc.File

	for _, f1 := range s1 {
		found := false
		for _, f2 := range s2 {
			if f1.OwnerGoogleID == f2.OwnerGoogleID && f1.FileName == f2.FileName {
				found = true
				break
			}
		}
		if !found {
			diff = append(diff, f1)
		}
	}

	return diff
}
