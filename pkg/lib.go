package pkg

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func GetEnvBool(name string, defaultValue bool) (bool, error) {
	rawValue := strings.TrimSpace(os.Getenv(name))
	if rawValue == "" {
		return defaultValue, nil
	}

	parsedValue, err := strconv.ParseBool(rawValue)
	if err != nil {
		return false, fmt.Errorf("invalid %s value %q: %w", name, rawValue, err)
	}

	return parsedValue, nil
}

func GetUserBucketName(bucketBaseName, userID string) string {
	return fmt.Sprintf("%s-%s", bucketBaseName, userID)
}

func ExtractUserIdFromBucketName(baseName, compositeName string) string {
	prefix := baseName + "-"
	if len(compositeName) > len(prefix) && compositeName[:len(prefix)] == prefix {
		return compositeName[len(prefix):]
	}
	return ""
}

func LoadServiceAccount(path string) (email string, privateKey []byte, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}

	var creds struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", nil, err
	}

	if creds.ClientEmail == "" || creds.PrivateKey == "" {
		return "", nil, fmt.Errorf("invalid service account file: missing email or private key")
	}

	return creds.ClientEmail, []byte(creds.PrivateKey), nil
}

func RandToken(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func GenerateSecureTokenFromID(id int64) (string, error) {
	// Convert the ID to string
	idStr := strconv.FormatInt(id, 10)

	// Generate 32 random bytes
	randBytes := make([]byte, 32)
	_, err := rand.Read(randBytes)
	if err != nil {
		return "", err
	}

	// Mix the ID and the random bytes together
	combined := append([]byte(idStr), randBytes...)

	// Hash the result to create a fixed-length string
	hash := sha256.Sum256(combined)

	// Convert to a 64-character hex string
	return hex.EncodeToString(hash[:]), nil
}

func GenerateSecureTokenFromIDStr(idStr string) (string, error) {
	// Generate 32 random bytes
	randBytes := make([]byte, 32)
	_, err := rand.Read(randBytes)
	if err != nil {
		return "", err
	}

	// Mix the ID and the random bytes together
	combined := append([]byte(idStr), randBytes...)

	// Hash the result to create a fixed-length string
	hash := sha256.Sum256(combined)

	// Convert to a 64-character hex string
	return hex.EncodeToString(hash[:]), nil
}

func CustomParseDuration(s string) (time.Duration, error) {
	s = trimSpaces(s)
	if s == "" {
		return 0, errors.New("empty string")
	}

	// Split manually: find where the number ends and letters begin
	var numPart, unitPart string
	for i, r := range s {
		if unicode.IsLetter(r) {
			numPart = s[:i]
			unitPart = s[i:]
			break
		}
	}

	if numPart == "" || unitPart == "" {
		return 0, errors.New("invalid format: must have number and unit")
	}

	num, err := strconv.Atoi(numPart)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %w", err)
	}

	switch unitPart {
	case "ns", "us", "µs", "ms", "s", "m", "h":
		return time.ParseDuration(fmt.Sprintf("%d%s", num, unitPart))
	case "d":
		return time.Duration(num) * 24 * time.Hour, nil
	case "w":
		return time.Duration(num) * 7 * 24 * time.Hour, nil
	case "mo":
		return time.Duration(num) * 30 * 24 * time.Hour, nil
	case "y":
		return time.Duration(num) * 365 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown unit: %s", unitPart)
	}
}

// helper to trim leading and trailing spaces
func trimSpaces(s string) string {
	start, end := 0, len(s)
	for start < end && unicode.IsSpace(rune(s[start])) {
		start++
	}
	for start < end && unicode.IsSpace(rune(s[end-1])) {
		end--
	}
	return s[start:end]
}

func JSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(v); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		JSON(w, map[string]any{
			"status":   http.StatusInternalServerError,
			"response": "Error encoding JSON",
		})
		return
	}
}

func WriteJSONResponse(w http.ResponseWriter, responseStatus int, customMessage string, customResponse any) {
	w.WriteHeader(responseStatus)

	resp := map[string]any{
		"status":   responseStatus,
		"response": customResponse,
	}
	if customMessage != "" {
		resp["msg"] = customMessage
	}

	JSON(w, resp)
}

func ReadConfigRequired(envVar string) string {
	value := os.Getenv(envVar)
	if value == "" {
		panic(envVar + " is required")
	}
	return value
}
