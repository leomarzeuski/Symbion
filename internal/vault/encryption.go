package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/pbkdf2"
)

const (
	currentEncryptedProfileVersion = 2
	legacyPBKDF2Iterations         = 210000
	argon2Time                     = 3
	argon2MemoryKiB                = 64 * 1024
	argon2Parallelism              = 4
	keySize                        = 32
	saltSize                       = 16
	nonceSize                      = 12
)

type encryptedProfileFile struct {
	Version     int    `json:"version"`
	KDF         string `json:"kdf"`
	Iterations  int    `json:"iterations,omitempty"`
	Memory      uint32 `json:"memory,omitempty"`
	Time        uint32 `json:"time,omitempty"`
	Parallelism uint8  `json:"parallelism,omitempty"`
	Salt        string `json:"salt"`
	Nonce       string `json:"nonce"`
	Data        string `json:"data"`
}

func Encrypt(plain []byte, passphrase []byte) ([]byte, error) {
	if len(passphrase) == 0 {
		return nil, ErrPassphraseRequired
	}

	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	key := argon2.IDKey(passphrase, salt, argon2Time, argon2MemoryKiB, argon2Parallelism, keySize)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	file := encryptedProfileFile{
		Version:     currentEncryptedProfileVersion,
		KDF:         "argon2id",
		Memory:      argon2MemoryKiB,
		Time:        argon2Time,
		Parallelism: argon2Parallelism,
		Salt:        base64.StdEncoding.EncodeToString(salt),
		Nonce:       base64.StdEncoding.EncodeToString(nonce),
		Data:        base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, plain, nil)),
	}

	return json.MarshalIndent(file, "", "  ")
}

func Decrypt(encrypted []byte, passphrase []byte) ([]byte, error) {
	if len(passphrase) == 0 {
		return nil, ErrPassphraseRequired
	}

	var file encryptedProfileFile
	if err := json.Unmarshal(encrypted, &file); err != nil {
		return nil, fmt.Errorf("parse encrypted profile: %w", err)
	}
	if file.Version < 1 || file.Version > currentEncryptedProfileVersion {
		return nil, fmt.Errorf("unsupported encrypted profile version %d", file.Version)
	}
	if file.KDF != "argon2id" && file.KDF != "pbkdf2-sha256" {
		return nil, fmt.Errorf("unsupported encrypted profile kdf %q", file.KDF)
	}

	salt, err := base64.StdEncoding.DecodeString(file.Salt)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted profile salt: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(file.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted profile nonce: %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(file.Data)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted profile data: %w", err)
	}

	key, err := deriveKey(file, passphrase, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("encrypted profile has invalid nonce size")
	}

	plain, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt encrypted profile: %w", err)
	}

	return plain, nil
}

func deriveKey(file encryptedProfileFile, passphrase []byte, salt []byte) ([]byte, error) {
	switch file.KDF {
	case "argon2id":
		if file.Memory == 0 || file.Time == 0 || file.Parallelism == 0 {
			return nil, fmt.Errorf("encrypted profile has invalid argon2id parameters")
		}
		return argon2.IDKey(passphrase, salt, file.Time, file.Memory, file.Parallelism, keySize), nil
	case "pbkdf2-sha256":
		if file.Iterations <= 0 {
			return nil, fmt.Errorf("encrypted profile has invalid iteration count")
		}
		return pbkdf2.Key(passphrase, salt, file.Iterations, keySize, sha256.New), nil
	default:
		return nil, fmt.Errorf("unsupported encrypted profile kdf %q", file.KDF)
	}
}
