package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

func TestSaveListAndUseProfile(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	envPath := filepath.Join(projectRoot, ".env")
	original := []byte("DATABASE_URL=postgres://local\n# keep comments\nAPI_KEY=secret\n")
	if err := os.WriteFile(envPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}

	store := Store{Root: root}
	path, err := store.Save("Billing API", "local", envPath)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if path != filepath.Join(root, "projects", "billing-api", "profiles", "local.env") {
		t.Fatalf("path = %q, want project profile path", path)
	}

	profiles, err := store.List("Billing API")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(profiles) != 1 || profiles[0].Name != "local" {
		t.Fatalf("profiles = %#v, want local", profiles)
	}

	if err := os.WriteFile(envPath, []byte("DATABASE_URL=changed\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(changed .env) error = %v", err)
	}

	if _, err := store.Use("Billing API", "local", envPath); err != nil {
		t.Fatalf("Use() error = %v", err)
	}

	restored, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile(.env) error = %v", err)
	}
	if !reflect.DeepEqual(restored, original) {
		t.Fatalf("restored .env = %q, want %q", restored, original)
	}
}

func TestListProfilesSorted(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root}

	dir := store.ProfileDir("billing-api")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	for _, name := range []string{"staging.env", "local.env", "README.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("KEY=value\n"), 0o600); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", name, err)
		}
	}

	profiles, err := store.List("billing-api")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	got := []string{}
	for _, profile := range profiles {
		got = append(got, profile.Name)
	}
	want := []string{"local", "staging"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("profiles = %#v, want %#v", got, want)
	}
}

func TestProjectLockPreventsConcurrentWrites(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	envPath := filepath.Join(projectRoot, ".env")
	if err := os.WriteFile(envPath, []byte("DATABASE_URL=postgres://local\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}

	store := Store{Root: root}
	unlock, err := store.lockProject("Billing API")
	if err != nil {
		t.Fatalf("lockProject() error = %v", err)
	}

	if _, err := store.Save("Billing API", "local", envPath); err == nil {
		t.Fatal("Save() while locked error = nil, want error")
	}

	unlock()
	if _, err := store.Save("Billing API", "local", envPath); err != nil {
		t.Fatalf("Save() after unlock error = %v", err)
	}
}

func TestWritePrivateFileReplacesContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := writePrivateFile(path, []byte("API_KEY=one\n")); err != nil {
		t.Fatalf("writePrivateFile() error = %v", err)
	}
	if err := writePrivateFile(path, []byte("API_KEY=two\n")); err != nil {
		t.Fatalf("writePrivateFile() replace error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "API_KEY=two\n" {
		t.Fatalf("data = %q, want replacement content", data)
	}
}

func TestSaveEncryptedRemovesPlainProfile(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	envPath := filepath.Join(projectRoot, ".env")
	original := []byte("DATABASE_URL=postgres://local\nAPI_KEY=secret\n")
	if err := os.WriteFile(envPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}

	store := Store{Root: root}
	if _, err := store.Save("Billing API", "local", envPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	path, err := store.SaveEncrypted("Billing API", "local", envPath, []byte("passphrase"))
	if err != nil {
		t.Fatalf("SaveEncrypted() error = %v", err)
	}
	if filepath.Ext(path) != ".enc" {
		t.Fatalf("encrypted path = %q, want .enc suffix", path)
	}
	if _, err := os.Stat(store.ProfilePath("Billing API", "local")); !os.IsNotExist(err) {
		t.Fatalf("plain profile still exists after encrypted save")
	}

	profiles, err := store.List("Billing API")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(profiles) != 1 || !profiles[0].Encrypted {
		t.Fatalf("profiles = %#v, want encrypted local", profiles)
	}

	data, profile, err := store.ReadProfile("Billing API", "local", []byte("passphrase"))
	if err != nil {
		t.Fatalf("ReadProfile() error = %v", err)
	}
	if !profile.Encrypted {
		t.Fatalf("profile.Encrypted = false, want true")
	}
	if !reflect.DeepEqual(data, original) {
		t.Fatalf("decrypted data = %q, want %q", data, original)
	}
}

func TestUseProfileCreatesBackup(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	envPath := filepath.Join(projectRoot, ".env")
	if err := os.WriteFile(envPath, []byte("DATABASE_URL=old\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}

	store := Store{Root: root}
	profileEnv := filepath.Join(projectRoot, "profile.env")
	if err := os.WriteFile(profileEnv, []byte("DATABASE_URL=new\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(profile.env) error = %v", err)
	}
	if _, err := store.Save("Billing API", "local", profileEnv); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	now := time.Date(2026, 4, 25, 16, 30, 0, 0, time.UTC)
	result, err := store.UseProfile("Billing API", "local", envPath, nil, now)
	if err != nil {
		t.Fatalf("UseProfile() error = %v", err)
	}
	if !result.BackupCreated {
		t.Fatal("expected UseProfile to create a backup")
	}
	if result.Backup.Name != "20260425-163000-before-use-local.env" {
		t.Fatalf("Backup.Name = %q, want timestamped backup", result.Backup.Name)
	}

	backupData, err := os.ReadFile(result.Backup.Path)
	if err != nil {
		t.Fatalf("ReadFile(backup) error = %v", err)
	}
	if string(backupData) != "DATABASE_URL=old\n" {
		t.Fatalf("backup = %q, want old env", backupData)
	}

	current, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile(.env) error = %v", err)
	}
	if string(current) != "DATABASE_URL=new\n" {
		t.Fatalf(".env = %q, want new env", current)
	}
}

func TestUndoRestoresLatestBackup(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	envPath := filepath.Join(projectRoot, ".env")
	if err := os.WriteFile(envPath, []byte("DATABASE_URL=current\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}

	store := Store{Root: root}
	if err := os.MkdirAll(store.BackupDir("Billing API"), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	oldBackup := filepath.Join(store.BackupDir("Billing API"), "20260425-150000-before-use-local.env")
	latestBackup := filepath.Join(store.BackupDir("Billing API"), "20260425-160000-before-use-local.env")
	if err := os.WriteFile(oldBackup, []byte("DATABASE_URL=old\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(old backup) error = %v", err)
	}
	if err := os.WriteFile(latestBackup, []byte("DATABASE_URL=restored\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(latest backup) error = %v", err)
	}

	now := time.Date(2026, 4, 25, 17, 0, 0, 0, time.UTC)
	restored, currentBackup, created, err := store.Undo("Billing API", envPath, now)
	if err != nil {
		t.Fatalf("Undo() error = %v", err)
	}
	if restored.Path != latestBackup {
		t.Fatalf("restored = %q, want latest backup", restored.Path)
	}
	if !created || currentBackup.Name != "20260425-170000-before-undo.env" {
		t.Fatalf("current backup = %#v, created = %v", currentBackup, created)
	}

	current, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile(.env) error = %v", err)
	}
	if string(current) != "DATABASE_URL=restored\n" {
		t.Fatalf(".env = %q, want restored env", current)
	}
}

func TestEncryptDecrypt(t *testing.T) {
	plain := []byte("API_KEY=secret\n")
	passphrase := []byte("passphrase")

	encrypted, err := Encrypt(plain, passphrase)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if reflect.DeepEqual(encrypted, plain) {
		t.Fatal("encrypted data should not equal plain data")
	}

	decrypted, err := Decrypt(encrypted, passphrase)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if !reflect.DeepEqual(decrypted, plain) {
		t.Fatalf("decrypted = %q, want %q", decrypted, plain)
	}

	if _, err := Decrypt(encrypted, []byte("wrong")); err == nil {
		t.Fatal("Decrypt() with wrong passphrase error = nil, want error")
	}
}

func TestDecryptLegacyPBKDF2Profile(t *testing.T) {
	plain := []byte("API_KEY=legacy\n")
	passphrase := []byte("passphrase")

	encrypted, err := encryptLegacyPBKDF2ForTest(plain, passphrase)
	if err != nil {
		t.Fatalf("encryptLegacyPBKDF2ForTest() error = %v", err)
	}

	decrypted, err := Decrypt(encrypted, passphrase)
	if err != nil {
		t.Fatalf("Decrypt(legacy) error = %v", err)
	}
	if !reflect.DeepEqual(decrypted, plain) {
		t.Fatalf("decrypted = %q, want %q", decrypted, plain)
	}
}

func TestValidateProfileName(t *testing.T) {
	valid := []string{"local", "staging-1", "dev.local", "qa_team"}
	for _, name := range valid {
		if err := ValidateProfileName(name); err != nil {
			t.Fatalf("ValidateProfileName(%q) error = %v", name, err)
		}
	}

	invalid := []string{"", ".env", "../prod", "prod/local", "local env"}
	for _, name := range invalid {
		if err := ValidateProfileName(name); err == nil {
			t.Fatalf("ValidateProfileName(%q) error = nil, want error", name)
		}
	}
}

func TestProjectID(t *testing.T) {
	tests := map[string]string{
		"Billing API":      "billing-api",
		"  Project_2  ":    "project_2",
		"api/service prod": "api-service-prod",
		"***":              "project",
	}

	for input, want := range tests {
		if got := ProjectID(input); got != want {
			t.Fatalf("ProjectID(%q) = %q, want %q", input, got, want)
		}
	}
}

func encryptLegacyPBKDF2ForTest(plain []byte, passphrase []byte) ([]byte, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	key := pbkdf2.Key(passphrase, salt, legacyPBKDF2Iterations, keySize, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return json.Marshal(encryptedProfileFile{
		Version:    1,
		KDF:        "pbkdf2-sha256",
		Iterations: legacyPBKDF2Iterations,
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Data:       base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, plain, nil)),
	})
}
