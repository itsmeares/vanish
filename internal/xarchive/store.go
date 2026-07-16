package xarchive

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	archiveDirName = "x-archives"
	manifestName   = "manifest.json"
	indexName      = "index.jsonl"
	postsName      = "posts.jsonl"
	maxIndexLine   = 1 << 20
)

type Store struct {
	workspaceDir string
	root         string
}

type Dataset struct {
	root     string
	manifest DatasetManifest
	index    []IndexEntry
}

func NewStore(workspaceDir string) *Store {
	clean := filepath.Clean(strings.TrimSpace(workspaceDir))
	return &Store{workspaceDir: clean, root: filepath.Join(clean, archiveDirName)}
}

func (s *Store) Root() string { return s.root }

func (s *Store) List() ([]DatasetSummary, error) {
	if err := validateDirectory(s.root); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	summaries := make([]DatasetSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validSHA256(entry.Name()) {
			continue
		}
		manifest, err := readManifest(filepath.Join(s.root, entry.Name()))
		if err != nil || manifest.DatasetID != entry.Name() || validateManifest(manifest) != nil {
			continue
		}
		summaries = append(summaries, summaryFromManifest(manifest))
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].ImportedAt.Equal(summaries[j].ImportedAt) {
			return summaries[i].DatasetID < summaries[j].DatasetID
		}
		return summaries[i].ImportedAt.After(summaries[j].ImportedAt)
	})
	return summaries, nil
}

func (s *Store) Open(id string) (*Dataset, error) {
	id = strings.TrimSpace(id)
	if !validSHA256(id) {
		return nil, errors.New("invalid X archive dataset ID")
	}
	root := filepath.Join(s.root, id)
	if err := validateDirectory(s.root); err != nil {
		return nil, err
	}
	if err := validateDirectory(root); err != nil {
		return nil, err
	}
	manifest, err := readManifest(root)
	if err != nil {
		return nil, err
	}
	if manifest.DatasetID != id || manifest.FormatVersion != DatasetFormatVersion {
		return nil, errors.New("unsupported X archive dataset")
	}
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}
	postsPath := filepath.Join(root, postsName)
	if err := validateRegularFile(postsPath); err != nil {
		return nil, err
	}
	postsInfo, err := os.Stat(postsPath)
	if err != nil || postsInfo.Size() != manifest.PostBytes {
		return nil, errors.New("X archive posts integrity check failed")
	}
	index, digest, bytesRead, err := readIndex(filepath.Join(root, indexName), manifest.PostBytes)
	if err != nil {
		return nil, err
	}
	if digest != manifest.IndexSHA256 || bytesRead != manifest.IndexBytes || len(index) != manifest.Counts.Total {
		return nil, errors.New("X archive index integrity check failed")
	}
	return &Dataset{root: root, manifest: manifest, index: index}, nil
}

func (d *Dataset) Summary() DatasetSummary { return summaryFromManifest(d.manifest) }
func (d *Dataset) Len() int                { return len(d.index) }

func (d *Dataset) Entry(index int) (IndexEntry, bool) {
	if index < 0 || index >= len(d.index) {
		return IndexEntry{}, false
	}
	return d.index[index], true
}

func (d *Dataset) ActivityAt(index int) (Activity, error) {
	entry, ok := d.Entry(index)
	if !ok {
		return Activity{}, errors.New("X archive activity is out of range")
	}
	postsPath := filepath.Join(d.root, postsName)
	if err := validateRegularFile(postsPath); err != nil {
		return Activity{}, err
	}
	file, err := os.Open(postsPath)
	if err != nil {
		return Activity{}, err
	}
	defer file.Close()
	record := make([]byte, entry.Length)
	if _, err := file.ReadAt(record, entry.Offset); err != nil {
		return Activity{}, errors.New("X archive activity could not be read")
	}
	sum := sha256.Sum256(record)
	if hex.EncodeToString(sum[:]) != entry.RecordSHA256 {
		return Activity{}, errors.New("X archive activity integrity check failed")
	}
	var activity Activity
	if err := json.Unmarshal(record, &activity); err != nil || activity.ID != entry.ID {
		return Activity{}, errors.New("X archive activity is invalid")
	}
	return activity, nil
}

func (d *Dataset) ResolveMedia(ref MediaRef) (string, error) {
	path, err := d.mediaPath(ref)
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("X archive media is unavailable")
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || hex.EncodeToString(hash.Sum(nil)) != ref.SHA256 {
		return "", errors.New("X archive media integrity check failed")
	}
	return path, nil
}

func (d *Dataset) MediaAvailable(ref MediaRef) bool {
	_, err := d.mediaPath(ref)
	return err == nil
}

func (d *Dataset) mediaPath(ref MediaRef) (string, error) {
	if !validSHA256(ref.SHA256) || ref.Bytes <= 0 || ref.Bytes > int64(maxMediaItemBytes) {
		return "", errors.New("invalid X archive media reference")
	}
	extension := ""
	switch {
	case ref.Kind == MediaPhoto && ref.MIME == "image/jpeg":
		extension = ".jpg"
	case ref.Kind == MediaPhoto && ref.MIME == "image/png":
		extension = ".png"
	case (ref.Kind == MediaVideo || ref.Kind == MediaAnimation) && ref.MIME == "video/mp4":
		extension = ".mp4"
	default:
		return "", errors.New("invalid X archive media reference")
	}
	expected := path.Join("media", ref.SHA256[:2], ref.SHA256+extension)
	if ref.RelativePath != expected {
		return "", errors.New("invalid X archive media reference")
	}
	clean := filepath.Clean(filepath.FromSlash(ref.RelativePath))
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid X archive media path")
	}
	path := filepath.Join(d.root, clean)
	rel, err := filepath.Rel(d.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid X archive media path")
	}
	if err := validatePathComponents(d.root, path); err != nil {
		return "", errors.New("X archive media is unavailable")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != ref.Bytes {
		return "", errors.New("X archive media is unavailable")
	}
	return path, nil
}

func readManifest(root string) (DatasetManifest, error) {
	path := filepath.Join(root, manifestName)
	if err := validateRegularFile(path); err != nil {
		return DatasetManifest{}, err
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() <= 0 || info.Size() > 1<<20 {
		return DatasetManifest{}, errors.New("X archive manifest is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return DatasetManifest{}, err
	}
	defer file.Close()
	var manifest DatasetManifest
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	if err := decoder.Decode(&manifest); err != nil {
		return DatasetManifest{}, errors.New("X archive manifest is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return DatasetManifest{}, errors.New("X archive manifest is invalid")
	}
	return manifest, nil
}

func readIndex(path string, postBytes int64) ([]IndexEntry, string, int64, error) {
	if err := validateRegularFile(path); err != nil {
		return nil, "", 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	reader := bufio.NewReaderSize(io.TeeReader(file, hash), 64*1024)
	entries := make([]IndexEntry, 0)
	var total int64
	for {
		line, err := reader.ReadBytes('\n')
		total += int64(len(line))
		if len(line) > maxIndexLine {
			return nil, "", 0, errors.New("X archive index line is too large")
		}
		trimmed := strings.TrimSpace(string(line))
		if trimmed != "" {
			var entry IndexEntry
			if json.Unmarshal([]byte(trimmed), &entry) != nil || !validSHA256(entry.ID) || !validSHA256(entry.RecordSHA256) || entry.Offset < 0 || entry.Length <= 0 || entry.Length > maxNormalizedRecord || entry.Offset > postBytes-entry.Length {
				return nil, "", 0, errors.New("X archive index is invalid")
			}
			entries = append(entries, entry)
			if len(entries) > maxActivities {
				return nil, "", 0, errors.New("X archive activity limit exceeded")
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", 0, err
		}
	}
	return entries, hex.EncodeToString(hash.Sum(nil)), total, nil
}

func summaryFromManifest(manifest DatasetManifest) DatasetSummary {
	return DatasetSummary{
		DatasetID: manifest.DatasetID, Account: manifest.Account, ImportedAt: manifest.ImportedAt,
		Demo: manifest.Demo, Counts: manifest.Counts, WarningCount: manifest.WarningCount,
		StoredBytes: manifest.PostBytes + manifest.IndexBytes + manifest.MediaBytes,
	}
}

func validateManifest(manifest DatasetManifest) error {
	if manifest.FormatVersion != DatasetFormatVersion || !validSHA256(manifest.DatasetID) || manifest.Counts.Total < 0 || manifest.Counts.Total > maxActivities || manifest.Counts.Total != manifest.Counts.Posts+manifest.Counts.Replies+manifest.Counts.QuotePosts+manifest.Counts.Reposts || manifest.PostBytes < 0 || manifest.IndexBytes < 0 || manifest.MediaBytes < 0 {
		return errors.New("X archive manifest is invalid")
	}
	if !validPostID(manifest.Account.ID) || manifest.Account.Username == "" || len(manifest.Account.Username) > 256 || len(manifest.Account.DisplayName) > 512 || manifest.Counts.Media < 0 || manifest.WarningCount < 0 {
		return errors.New("X archive manifest is invalid")
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("write X archive metadata: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write X archive metadata: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync X archive metadata: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close X archive metadata: %w", err)
	}
	return nil
}

func validateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("X archive storage path is unsafe")
	}
	return nil
}

func validateRegularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("X archive storage file is unsafe")
	}
	return nil
}

func validatePathComponents(root, target string) error {
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("X archive storage path is unsafe")
	}
	current := root
	if err := validateDirectory(current); err != nil {
		return err
	}
	parts := strings.Split(relative, string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("X archive storage path is unsafe")
		}
		if index < len(parts)-1 && !info.IsDir() {
			return errors.New("X archive storage path is unsafe")
		}
	}
	return nil
}
