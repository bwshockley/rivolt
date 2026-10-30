package videos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const MaxFileSize int64 = 1 << 30
const DefaultUserQuota int64 = 100 << 30

var (
	ErrDriveNotFound = errors.New("drive not found")
	ErrVideoNotFound = errors.New("video not found")
	ErrVideoTooLarge = errors.New("video exceeds the 1 GiB upload limit")
	ErrQuotaExceeded = errors.New("user video storage quota exceeded")
)

type Video struct {
	ID         string    `json:"id"`
	Camera     string    `json:"camera"`
	Sequence   int       `json:"sequence"`
	Filename   string    `json:"filename"`
	RecordedAt time.Time `json:"recorded_at"`
	SizeBytes  int64     `json:"size_bytes"`
	UploadedAt time.Time `json:"uploaded_at"`
	path       string
}

type Store struct {
	db    *sql.DB
	root  string
	quota int64
	mu    sync.Mutex
}

func NewStore(d *sql.DB, root string, quotaBytes ...int64) *Store {
	quota := DefaultUserQuota
	if len(quotaBytes) > 0 {
		quota = quotaBytes[0]
	}
	return &Store{db: d, root: root, quota: quota}
}

func (s *Store) List(ctx context.Context, userID uuid.UUID, driveID string) ([]Video, error) {
	drivePK, _, err := s.drivePK(ctx, userID, driveID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, camera, sequence, filename, recorded_at, size_bytes, uploaded_at, path
		FROM drive_videos WHERE user_id = $1 AND drive_id = $2
		ORDER BY camera, sequence`, userID, drivePK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Video
	for rows.Next() {
		var v Video
		if err := rows.Scan(&v.ID, &v.Camera, &v.Sequence, &v.Filename, &v.RecordedAt, &v.SizeBytes, &v.UploadedAt, &v.path); err != nil {
			return nil, err
		}
		v.RecordedAt = v.RecordedAt.UTC()
		v.UploadedAt = v.UploadedAt.UTC()
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) Upload(ctx context.Context, userID uuid.UUID, driveID, filename, timezone string, src io.Reader) (Video, error) {
	parsed, err := ParseFilename(filename, timezone)
	if err != nil {
		return Video{}, err
	}
	drivePK, driveStartedAt, err := s.drivePK(ctx, userID, driveID)
	if err != nil {
		return Video{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	videoID := uuid.New()
	driveFolder := driveStartedAt.In(parsed.RecordedAt.Location()).Format("20060102_150405") + "_" + drivePK.String()
	dir := filepath.Join(s.root, userID.String(), driveFolder)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Video{}, fmt.Errorf("create video directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return Video{}, fmt.Errorf("secure video directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".upload-*.tmp")
	if err != nil {
		return Video{}, fmt.Errorf("create upload file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return Video{}, fmt.Errorf("secure upload file: %w", err)
	}
	size, err := io.Copy(tmp, io.LimitReader(src, MaxFileSize+1))
	if err != nil {
		_ = tmp.Close()
		return Video{}, fmt.Errorf("write upload file: %w", err)
	}
	if size > MaxFileSize {
		_ = tmp.Close()
		return Video{}, ErrVideoTooLarge
	}
	if size < 12 {
		_ = tmp.Close()
		return Video{}, fmt.Errorf("%w: file is not an MP4 video", ErrInvalidVideo)
	}
	var signature [12]byte
	if _, err := tmp.ReadAt(signature[:], 0); err != nil || string(signature[4:8]) != "ftyp" {
		_ = tmp.Close()
		return Video{}, fmt.Errorf("%w: file is not an MP4 video", ErrInvalidVideo)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return Video{}, fmt.Errorf("sync upload file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return Video{}, fmt.Errorf("close upload file: %w", err)
	}
	if s.quota > 0 {
		var used int64
		err := s.db.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(size_bytes), 0) FROM drive_videos
			WHERE user_id = $1 AND NOT (drive_id = $2 AND camera = $3 AND sequence = $4)`,
			userID, drivePK, parsed.Camera, parsed.Sequence).Scan(&used)
		if err != nil {
			return Video{}, fmt.Errorf("check video storage quota: %w", err)
		}
		if size > s.quota-used {
			return Video{}, ErrQuotaExceeded
		}
	}
	path := filepath.Join(dir, videoID.String()+".mp4")
	if err := os.Rename(tmpName, path); err != nil {
		return Video{}, fmt.Errorf("finalize upload file: %w", err)
	}
	relativePath, err := filepath.Rel(s.root, path)
	if err != nil {
		_ = os.Remove(path)
		return Video{}, fmt.Errorf("resolve video path: %w", err)
	}

	var oldPath string
	err = s.db.QueryRowContext(ctx, `
		SELECT path FROM drive_videos
		WHERE user_id = $1 AND drive_id = $2 AND camera = $3 AND sequence = $4`,
		userID, drivePK, parsed.Camera, parsed.Sequence).Scan(&oldPath)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		_ = os.Remove(path)
		return Video{}, fmt.Errorf("read existing video manifest: %w", err)
	}
	var saved Video
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO drive_videos (id, user_id, drive_id, camera, sequence, filename, recorded_at, size_bytes, path)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (drive_id, camera, sequence) DO UPDATE SET
			id = EXCLUDED.id, user_id = EXCLUDED.user_id, filename = EXCLUDED.filename,
			recorded_at = EXCLUDED.recorded_at, size_bytes = EXCLUDED.size_bytes,
			path = EXCLUDED.path, uploaded_at = NOW()
		RETURNING id, camera, sequence, filename, recorded_at, size_bytes, uploaded_at`,
		videoID, userID, drivePK, parsed.Camera, parsed.Sequence, filename, parsed.RecordedAt, size, filepath.ToSlash(relativePath)).
		Scan(&saved.ID, &saved.Camera, &saved.Sequence, &saved.Filename, &saved.RecordedAt, &saved.SizeBytes, &saved.UploadedAt)
	if err != nil {
		_ = os.Remove(path)
		return Video{}, fmt.Errorf("save video manifest: %w", err)
	}
	saved.RecordedAt = saved.RecordedAt.UTC()
	saved.UploadedAt = saved.UploadedAt.UTC()
	saved.path = filepath.ToSlash(relativePath)
	if oldPath != "" && filepath.Clean(oldPath) != filepath.Clean(relativePath) {
		_ = os.Remove(s.resolvePath(oldPath))
	}
	return saved, nil
}

func (s *Store) Open(ctx context.Context, userID uuid.UUID, driveID, videoID string) (*Video, *os.File, error) {
	drivePK, _, err := s.drivePK(ctx, userID, driveID)
	if err != nil {
		return nil, nil, err
	}
	parsedID, err := uuid.Parse(videoID)
	if err != nil {
		return nil, nil, ErrVideoNotFound
	}
	var v Video
	err = s.db.QueryRowContext(ctx, `
		SELECT id, camera, sequence, filename, recorded_at, size_bytes, uploaded_at, path
		FROM drive_videos WHERE user_id = $1 AND drive_id = $2 AND id = $3`,
		userID, drivePK, parsedID).Scan(&v.ID, &v.Camera, &v.Sequence, &v.Filename, &v.RecordedAt, &v.SizeBytes, &v.UploadedAt, &v.path)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrVideoNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Open(s.resolvePath(v.path))
	if err != nil {
		return nil, nil, err
	}
	return &v, file, nil
}

func (s *Store) Delete(ctx context.Context, userID uuid.UUID, driveID, videoID string) error {
	drivePK, _, err := s.drivePK(ctx, userID, driveID)
	if err != nil {
		return err
	}
	parsedID, err := uuid.Parse(videoID)
	if err != nil {
		return ErrVideoNotFound
	}
	var path string
	err = s.db.QueryRowContext(ctx, `
		DELETE FROM drive_videos WHERE user_id = $1 AND drive_id = $2 AND id = $3
		RETURNING path`, userID, drivePK, parsedID).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrVideoNotFound
	}
	if err != nil {
		return err
	}
	if err := os.Remove(s.resolvePath(path)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete video file: %w", err)
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM drive_videos WHERE user_id = $1`, userID); err != nil {
		return err
	}
	return s.RemoveUserFiles(userID)
}

func (s *Store) RemoveUserFiles(userID uuid.UUID) error {
	if err := os.RemoveAll(filepath.Join(s.root, userID.String())); err != nil {
		return fmt.Errorf("delete user video files: %w", err)
	}
	return nil
}

func (s *Store) drivePK(ctx context.Context, userID uuid.UUID, driveID string) (uuid.UUID, time.Time, error) {
	var id uuid.UUID
	var startedAt time.Time
	err := s.db.QueryRowContext(ctx, `SELECT id, started_at FROM drives WHERE user_id = $1 AND external_id = $2 LIMIT 1`, userID, driveID).Scan(&id, &startedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, time.Time{}, ErrDriveNotFound
	}
	return id, startedAt, err
}

func (s *Store) resolvePath(relative string) string {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return filepath.Join(s.root, ".invalid-video-path")
	}
	return filepath.Join(s.root, clean)
}
