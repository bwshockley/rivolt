package api

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/apohor/rivolt/internal/videos"
)

func handleDriveVideosList(store *videos.Store, uid uuid.UUID) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := store.List(r.Context(), uid, chi.URLParam(r, "id"))
		if err != nil {
			writeDriveVideoError(w, err)
			return
		}
		if items == nil {
			items = []videos.Video{}
		}
		writeJSON(w, http.StatusOK, items)
	}
}

func handleDriveVideoUpload(store *videos.Store, uid uuid.UUID) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(2 * time.Hour))
		r.Body = http.MaxBytesReader(w, r.Body, videos.MaxFileSize+(1<<20))
		reader, err := r.MultipartReader()
		if err != nil {
			http.Error(w, "expected multipart form upload", http.StatusBadRequest)
			return
		}
		driveID := chi.URLParam(r, "id")
		timezone := ""
		var saved *videos.Video
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				var tooLarge *http.MaxBytesError
				if errors.As(err, &tooLarge) {
					writeDriveVideoError(w, err)
				} else {
					http.Error(w, "invalid multipart upload", http.StatusBadRequest)
				}
				return
			}
			switch part.FormName() {
			case "timezone":
				if timezone != "" || saved != nil {
					_ = part.Close()
					http.Error(w, "timezone must appear once before the file", http.StatusBadRequest)
					return
				}
				value, err := io.ReadAll(io.LimitReader(part, 129))
				_ = part.Close()
				if err != nil || len(value) == 0 || len(value) > 128 {
					http.Error(w, "invalid timezone", http.StatusBadRequest)
					return
				}
				timezone = strings.TrimSpace(string(value))
			case "file":
				if saved != nil || timezone == "" || part.FileName() == "" {
					_ = part.Close()
					http.Error(w, "upload one named video after its timezone", http.StatusBadRequest)
					return
				}
				video, err := store.Upload(r.Context(), uid, driveID, part.FileName(), timezone, part)
				_ = part.Close()
				if err != nil {
					writeDriveVideoError(w, err)
					return
				}
				saved = &video
			default:
				_ = part.Close()
				http.Error(w, "unexpected upload field", http.StatusBadRequest)
				return
			}
		}
		if saved == nil {
			http.Error(w, "missing video file", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, saved)
	}
}

func handleDriveVideoContent(store *videos.Store, uid uuid.UUID) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		video, file, err := store.Open(r.Context(), uid, chi.URLParam(r, "id"), chi.URLParam(r, "videoID"))
		if err != nil {
			writeDriveVideoError(w, err)
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(2 * time.Hour))
		http.ServeContent(w, r, video.Filename, video.UploadedAt, file)
	}
}

func handleDriveVideoDelete(store *videos.Store, uid uuid.UUID) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.Delete(r.Context(), uid, chi.URLParam(r, "id"), chi.URLParam(r, "videoID")); err != nil {
			writeDriveVideoError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeDriveVideoError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, videos.ErrDriveNotFound), errors.Is(err, videos.ErrVideoNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, videos.ErrVideoTooLarge):
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
	case errors.Is(err, videos.ErrQuotaExceeded):
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
	case errors.Is(err, videos.ErrInvalidVideo):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "video exceeds the 1 GiB upload limit", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "video operation failed", http.StatusInternalServerError)
	}
}