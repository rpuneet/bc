// Package attachment provides file storage for channel attachments.
// Files are stored on the local filesystem under {stateDir}/attachments/{id}/{filename}.
// This is a stop-gap implementation; the storage backend can be swapped later.
package attachment

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// MaxFileSize is the default maximum file size (50MB).
const MaxFileSize = 50 * 1024 * 1024

// MaxFilesPerMessage is the default maximum number of attachments per message.
const MaxFilesPerMessage = 5

// allowedMIME is the whitelist of permitted MIME types.
var allowedMIME = map[string]bool{
	"image/jpeg":       true,
	"image/png":        true,
	"image/gif":        true,
	"image/webp":       true,
	"application/pdf":  true,
	"text/plain":       true,
	"application/json": true,
	"application/zip":  true,
	"application/gzip": true,
	"video/mp4":        true,
	"audio/mpeg":       true,
}

// Metadata holds information about a stored attachment.
type Metadata struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
	Filename  string    `json:"filename"`
	MIMEType  string    `json:"mime_type"`
	Channel   string    `json:"channel"`
	Sender    string    `json:"sender"`
	Size      int64     `json:"size"`
}

// Store manages attachment file storage on the local filesystem.
type Store struct {
	dir        string   // base directory for attachments
	sharedDirs []string // additional directories to search for files (e.g., /tmp/mycel-shared)
}

// NewStore creates an attachment store rooted at the given directory.
func NewStore(stateDir string) *Store {
	dir := filepath.Join(stateDir, "attachments")
	return &Store{dir: dir}
}

// AddSharedDir adds a directory to search when looking up files by name.
// Files in shared dirs are served by filename (not by hex ID).
func (s *Store) AddSharedDir(dir string) {
	s.sharedDirs = append(s.sharedDirs, dir)
}

// Save stores file data and returns metadata. The filename is sanitized.
func (s *Store) Save(data []byte, filename, channel, sender string) (*Metadata, error) {
	if len(data) > MaxFileSize {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", len(data), MaxFileSize)
	}

	// Detect and validate MIME type
	mimeType := detectMIME(data, filename)
	if !allowedMIME[mimeType] {
		return nil, fmt.Errorf("unsupported file type: %s", mimeType)
	}

	// Generate unique ID
	id := generateID()

	// Sanitize filename — strip path components, keep only base name
	safeName := sanitizeFilename(filename)
	if safeName == "" {
		safeName = "attachment"
	}

	// Create directory
	attachDir := filepath.Join(s.dir, id)
	if err := os.MkdirAll(attachDir, 0o750); err != nil {
		return nil, fmt.Errorf("create attachment dir: %w", err)
	}

	// Write file
	filePath := filepath.Join(attachDir, safeName)
	if err := os.WriteFile(filePath, data, 0o644); err != nil { //nolint:gosec // stored in controlled directory
		return nil, fmt.Errorf("write attachment: %w", err)
	}

	return &Metadata{
		ID:        id,
		Filename:  safeName,
		MIMEType:  mimeType,
		Size:      int64(len(data)),
		Channel:   channel,
		Sender:    sender,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// Get returns the file data and metadata for the given attachment ID.
// It first checks the attachments directory by hex ID, then searches
// shared directories by filename (for Playwright screenshots, etc.).
func (s *Store) Get(id string) ([]byte, *Metadata, error) {
	// Try hex ID lookup in attachments dir
	if isValidID(id) {
		attachDir := filepath.Join(s.dir, id)
		entries, err := os.ReadDir(attachDir)
		if err == nil && len(entries) > 0 {
			filename := entries[0].Name()
			filePath := filepath.Join(attachDir, filename)
			data, readErr := os.ReadFile(filePath) //nolint:gosec // path constructed from validated ID
			if readErr == nil {
				info, _ := entries[0].Info()
				var modTime time.Time
				if info != nil {
					modTime = info.ModTime()
				}
				return data, &Metadata{
					ID:        id,
					Filename:  filename,
					MIMEType:  detectMIME(data, filename),
					Size:      int64(len(data)),
					CreatedAt: modTime,
				}, nil
			}
		}
	}

	// Fall through: search shared directories by filename
	safeName := sanitizeFilename(id)
	for _, dir := range s.sharedDirs {
		filePath := filepath.Join(dir, safeName)
		data, err := os.ReadFile(filePath) //nolint:gosec // searched in controlled shared dirs
		if err != nil {
			continue
		}
		info, _ := os.Stat(filePath)
		var modTime time.Time
		if info != nil {
			modTime = info.ModTime()
		}
		return data, &Metadata{
			ID:        safeName,
			Filename:  safeName,
			MIMEType:  detectMIME(data, safeName),
			Size:      int64(len(data)),
			CreatedAt: modTime,
		}, nil
	}

	return nil, nil, fmt.Errorf("attachment not found: %s", id)
}

// Delete removes an attachment by ID.
func (s *Store) Delete(id string) error {
	if !isValidID(id) {
		return fmt.Errorf("invalid attachment ID")
	}
	return os.RemoveAll(filepath.Join(s.dir, id))
}

// generateID creates a random hex ID.
func generateID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// attachmentIDRe matches attachment IDs: lowercase hex, 1-64 chars.
var attachmentIDRe = regexp.MustCompile(`^[0-9a-f]{1,64}$`)

// isValidID checks that an ID is a hex string (no path traversal).
func isValidID(id string) bool {
	return attachmentIDRe.MatchString(id)
}

// sanitizeFilename strips directory components and dangerous characters.
func sanitizeFilename(name string) string {
	// Take only the base name
	name = filepath.Base(name)
	// Remove null bytes and traversal segments
	name = strings.ReplaceAll(name, "\x00", "")
	name = strings.ReplaceAll(name, "..", "")
	// Limit length
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}

// detectMIME detects MIME type from content and filename.
func detectMIME(data []byte, filename string) string {
	// Try content-based detection first
	mimeType := "application/octet-stream"
	if len(data) >= 512 {
		mimeType = http.DetectContentType(data[:512])
	} else if len(data) > 0 {
		mimeType = http.DetectContentType(data)
	}

	// Override with extension for known types (DetectContentType can be imprecise)
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png":
		mimeType = "image/png"
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	case ".gif":
		mimeType = "image/gif"
	case ".webp":
		mimeType = "image/webp"
	case ".pdf":
		mimeType = "application/pdf"
	case ".json":
		mimeType = "application/json"
	case ".txt":
		mimeType = "text/plain"
	case ".mp4":
		mimeType = "video/mp4"
	case ".mp3":
		mimeType = "audio/mpeg"
	case ".zip":
		mimeType = "application/zip"
	case ".gz", ".gzip":
		mimeType = "application/gzip"
	}

	return mimeType
}
