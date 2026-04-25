package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const DefaultDirname = ".symbion"

var profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

var ErrPassphraseRequired = errors.New("encrypted profile requires a passphrase")

type Store struct {
	Root string
}

type Profile struct {
	Name      string
	Path      string
	Size      int64
	Encrypted bool
}

type Backup struct {
	Name string
	Path string
	Size int64
}

type UseResult struct {
	Source        Profile
	Backup        Backup
	BackupCreated bool
}

type unlockFunc func()

func NewDefaultStore() (Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Store{}, err
	}

	return Store{Root: filepath.Join(home, DefaultDirname)}, nil
}

func (s Store) Save(project string, profile string, envPath string) (string, error) {
	unlock, err := s.lockProject(project)
	if err != nil {
		return "", err
	}
	defer unlock()

	if err := ValidateProfileName(profile); err != nil {
		return "", err
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%s not found; create it before saving a profile", filepath.Base(envPath))
		}
		return "", err
	}

	path := s.ProfilePath(project, profile)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := writePrivateFile(path, data); err != nil {
		return "", err
	}
	if err := removeIfExists(s.EncryptedProfilePath(project, profile)); err != nil {
		return "", err
	}

	return path, nil
}

func (s Store) SaveEncrypted(project string, profile string, envPath string, passphrase []byte) (string, error) {
	unlock, err := s.lockProject(project)
	if err != nil {
		return "", err
	}
	defer unlock()

	if len(passphrase) == 0 {
		return "", ErrPassphraseRequired
	}
	if err := ValidateProfileName(profile); err != nil {
		return "", err
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%s not found; create it before saving a profile", filepath.Base(envPath))
		}
		return "", err
	}

	encrypted, err := Encrypt(data, passphrase)
	if err != nil {
		return "", err
	}

	path := s.EncryptedProfilePath(project, profile)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := writePrivateFile(path, encrypted); err != nil {
		return "", err
	}
	if err := removeIfExists(s.ProfilePath(project, profile)); err != nil {
		return "", err
	}

	return path, nil
}

func (s Store) Use(project string, profile string, envPath string) (string, error) {
	unlock, err := s.lockProject(project)
	if err != nil {
		return "", err
	}
	defer unlock()

	data, ref, err := s.ReadProfile(project, profile, nil)
	if err != nil {
		return "", err
	}

	if err := writePrivateFile(envPath, data); err != nil {
		return "", err
	}

	return ref.Path, nil
}

func (s Store) UseProfile(project string, profile string, envPath string, passphrase []byte, now time.Time) (UseResult, error) {
	unlock, err := s.lockProject(project)
	if err != nil {
		return UseResult{}, err
	}
	defer unlock()

	data, ref, err := s.ReadProfile(project, profile, passphrase)
	if err != nil {
		return UseResult{}, err
	}

	backup, created, err := s.BackupCurrent(project, envPath, "before-use-"+profile, now)
	if err != nil {
		return UseResult{}, err
	}

	if err := writePrivateFile(envPath, data); err != nil {
		return UseResult{}, err
	}

	return UseResult{
		Source:        ref,
		Backup:        backup,
		BackupCreated: created,
	}, nil
}

func (s Store) ReadProfile(project string, profile string, passphrase []byte) ([]byte, Profile, error) {
	if err := ValidateProfileName(profile); err != nil {
		return nil, Profile{}, err
	}

	ref, err := s.ResolveProfile(project, profile)
	if err != nil {
		return nil, Profile{}, err
	}

	data, err := os.ReadFile(ref.Path)
	if err != nil {
		return nil, Profile{}, err
	}

	if ref.Encrypted {
		if len(passphrase) == 0 {
			return nil, Profile{}, ErrPassphraseRequired
		}
		data, err = Decrypt(data, passphrase)
		if err != nil {
			return nil, Profile{}, err
		}
	}

	return data, ref, nil
}

func (s Store) ResolveProfile(project string, profile string) (Profile, error) {
	if err := ValidateProfileName(profile); err != nil {
		return Profile{}, err
	}

	encryptedPath := s.EncryptedProfilePath(project, profile)
	if info, err := os.Stat(encryptedPath); err == nil {
		return Profile{Name: profile, Path: encryptedPath, Size: info.Size(), Encrypted: true}, nil
	} else if !os.IsNotExist(err) {
		return Profile{}, err
	}

	path := s.ProfilePath(project, profile)
	if info, err := os.Stat(path); err == nil {
		return Profile{Name: profile, Path: path, Size: info.Size()}, nil
	} else if !os.IsNotExist(err) {
		return Profile{}, err
	}

	return Profile{}, fmt.Errorf("profile %q not found for project %q", profile, project)
}

func (s Store) List(project string) ([]Profile, error) {
	dir := s.ProfileDir(project)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Profile{}, nil
		}
		return nil, err
	}

	byName := make(map[string]Profile, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name, encrypted, ok := profileNameFromFilename(entry.Name())
		if !ok {
			continue
		}
		if ValidateProfileName(name) != nil {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, err
		}

		existing, exists := byName[name]
		if exists && existing.Encrypted {
			continue
		}

		byName[name] = Profile{
			Name:      name,
			Path:      filepath.Join(dir, entry.Name()),
			Size:      info.Size(),
			Encrypted: encrypted,
		}
	}

	profiles := make([]Profile, 0, len(byName))
	for _, profile := range byName {
		profiles = append(profiles, profile)
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})

	return profiles, nil
}

func (s Store) BackupCurrent(project string, envPath string, label string, now time.Time) (Backup, bool, error) {
	data, err := os.ReadFile(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Backup{}, false, nil
		}
		return Backup{}, false, err
	}

	dir := s.BackupDir(project)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Backup{}, false, err
	}

	name := backupName(label, now)
	path := filepath.Join(dir, name)
	for i := 2; fileExists(path); i++ {
		name = strings.TrimSuffix(backupName(label, now), ".env") + fmt.Sprintf("-%d.env", i)
		path = filepath.Join(dir, name)
	}

	if err := writePrivateFile(path, data); err != nil {
		return Backup{}, false, err
	}

	return Backup{Name: name, Path: path, Size: int64(len(data))}, true, nil
}

func (s Store) ListBackups(project string) ([]Backup, error) {
	dir := s.BackupDir(project)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Backup{}, nil
		}
		return nil, err
	}

	backups := make([]Backup, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".env" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, err
		}

		backups = append(backups, Backup{
			Name: entry.Name(),
			Path: filepath.Join(dir, entry.Name()),
			Size: info.Size(),
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Name > backups[j].Name
	})

	return backups, nil
}

func (s Store) Undo(project string, envPath string, now time.Time) (Backup, Backup, bool, error) {
	unlock, err := s.lockProject(project)
	if err != nil {
		return Backup{}, Backup{}, false, err
	}
	defer unlock()

	backups, err := s.ListBackups(project)
	if err != nil {
		return Backup{}, Backup{}, false, err
	}
	if len(backups) == 0 {
		return Backup{}, Backup{}, false, fmt.Errorf("no backups found for project %q", project)
	}

	restore := backups[0]
	data, err := os.ReadFile(restore.Path)
	if err != nil {
		return Backup{}, Backup{}, false, err
	}

	currentBackup, created, err := s.BackupCurrent(project, envPath, "before-undo", now)
	if err != nil {
		return Backup{}, Backup{}, false, err
	}

	if err := writePrivateFile(envPath, data); err != nil {
		return Backup{}, Backup{}, false, err
	}

	return restore, currentBackup, created, nil
}

func (s Store) ProfilePath(project string, profile string) string {
	return filepath.Join(s.ProfileDir(project), profile+".env")
}

func (s Store) EncryptedProfilePath(project string, profile string) string {
	return filepath.Join(s.ProfileDir(project), profile+".env.enc")
}

func (s Store) ProfileDir(project string) string {
	return filepath.Join(s.Root, "projects", ProjectID(project), "profiles")
}

func (s Store) BackupDir(project string) string {
	return filepath.Join(s.Root, "projects", ProjectID(project), "backups")
}

func (s Store) LockPath(project string) string {
	return filepath.Join(s.Root, "projects", ProjectID(project), ".lock")
}

func ValidateProfileName(name string) error {
	if !profileNamePattern.MatchString(name) {
		return fmt.Errorf("invalid profile name %q; use letters, numbers, dots, dashes or underscores", name)
	}

	return nil
}

func ProjectID(project string) string {
	project = strings.TrimSpace(strings.ToLower(project))
	if project == "" {
		return "project"
	}

	var b strings.Builder
	previousDash := false
	for _, r := range project {
		valid := unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '-'
		if valid {
			b.WriteRune(r)
			previousDash = r == '-'
			continue
		}

		if !previousDash {
			b.WriteRune('-')
			previousDash = true
		}
	}

	id := strings.Trim(b.String(), "-")
	if id == "" {
		return "project"
	}

	return id
}

func profileNameFromFilename(name string) (string, bool, bool) {
	if strings.HasSuffix(name, ".env.enc") {
		return strings.TrimSuffix(name, ".env.enc"), true, true
	}
	if strings.HasSuffix(name, ".env") {
		return strings.TrimSuffix(name, ".env"), false, true
	}
	return "", false, false
}

func backupName(label string, now time.Time) string {
	label = sanitizeLabel(label)
	if label == "" {
		label = "backup"
	}
	return now.Format("20060102-150405") + "-" + label + ".env"
}

func sanitizeLabel(label string) string {
	label = strings.TrimSpace(strings.ToLower(label))
	if label == "" {
		return ""
	}

	var b strings.Builder
	previousDash := false
	for _, r := range label {
		valid := unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' || r == '-'
		if valid {
			b.WriteRune(r)
			previousDash = r == '-'
			continue
		}

		if !previousDash {
			b.WriteRune('-')
			previousDash = true
		}
	}

	return strings.Trim(b.String(), "-")
}

func writePrivateFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}

	return syncDir(dir)
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s Store) lockProject(project string) (unlockFunc, error) {
	lockPath := s.LockPath(project)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("project %q is locked; another symbion command may be running", project)
		}
		return nil, err
	}

	_, writeErr := fmt.Fprintf(file, "pid=%d\ncreated_at=%s\n", os.Getpid(), time.Now().Format(time.RFC3339))
	closeErr := file.Close()
	if writeErr != nil {
		os.Remove(lockPath)
		return nil, writeErr
	}
	if closeErr != nil {
		os.Remove(lockPath)
		return nil, closeErr
	}

	return func() {
		_ = os.Remove(lockPath)
	}, nil
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
