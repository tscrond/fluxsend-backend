package hash

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

type argon2idParams struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

var (
	errInvalidHash         = errors.New("invalid password hash format")
	errIncompatibleVersion = errors.New("incompatible argon2 version")
	defaultArgon2idParams  = argon2idParams{
		memory:      64 * 1024,
		iterations:  3,
		parallelism: 2,
		saltLength:  16,
		keyLength:   32,
	}
)

// HashPasswordArgon2id creates an encoded Argon2id password hash suitable for storage.
func HashPasswordArgon2id(password string) (string, error) {
	if password == "" {
		return "", errors.New("password cannot be empty")
	}

	salt := make([]byte, defaultArgon2idParams.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		defaultArgon2idParams.iterations,
		defaultArgon2idParams.memory,
		defaultArgon2idParams.parallelism,
		defaultArgon2idParams.keyLength,
	)

	b64 := base64.RawStdEncoding
	encodedSalt := b64.EncodeToString(salt)
	encodedHash := b64.EncodeToString(hash)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		defaultArgon2idParams.memory,
		defaultArgon2idParams.iterations,
		defaultArgon2idParams.parallelism,
		encodedSalt,
		encodedHash,
	), nil
}

// VerifyPasswordArgon2id verifies a plaintext password against an encoded Argon2id hash.
func VerifyPasswordArgon2id(password, encodedHash string) (bool, error) {
	if password == "" || encodedHash == "" {
		return false, nil
	}

	params, salt, decodedHash, err := parseArgon2idHash(encodedHash)
	if err != nil {
		return false, err
	}

	otherHash := argon2.IDKey(
		[]byte(password),
		salt,
		params.iterations,
		params.memory,
		params.parallelism,
		params.keyLength,
	)

	match := subtle.ConstantTimeCompare(decodedHash, otherHash) == 1
	return match, nil
}

func parseArgon2idHash(encodedHash string) (argon2idParams, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return argon2idParams{}, nil, nil, errInvalidHash
	}
	if parts[1] != "argon2id" {
		return argon2idParams{}, nil, nil, errInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return argon2idParams{}, nil, nil, errInvalidHash
	}
	if version != argon2.Version {
		return argon2idParams{}, nil, nil, errIncompatibleVersion
	}

	params := argon2idParams{}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.memory, &params.iterations, &params.parallelism); err != nil {
		return argon2idParams{}, nil, nil, errInvalidHash
	}

	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return argon2idParams{}, nil, nil, errInvalidHash
	}
	hash, err := b64.DecodeString(parts[5])
	if err != nil {
		return argon2idParams{}, nil, nil, errInvalidHash
	}
	params.saltLength = uint32(len(salt))
	params.keyLength = uint32(len(hash))

	if params.memory == 0 || params.iterations == 0 || params.parallelism == 0 || params.keyLength == 0 {
		return argon2idParams{}, nil, nil, errInvalidHash
	}

	return params, salt, hash, nil
}
