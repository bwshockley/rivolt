package videos

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

var videoFilenamePattern = regexp.MustCompile(`^(\d{2})_(\d{2})_(\d{2})_(\d{6})_(frontCenter|rearCenter|sideLeft|sideRight)_(\d{3})\.mp4$`)

var ErrInvalidVideo = errors.New("invalid Road Cam video")

// Filename describes the drive timestamp and order encoded in a Road Cam clip name.
type Filename struct {
	Camera     string
	Sequence   int
	RecordedAt time.Time
}

// ParseFilename parses a Road Cam filename using the timezone in which it was recorded.
func ParseFilename(name, timezone string) (Filename, error) {
	if filepath.Base(name) != name {
		return Filename{}, fmt.Errorf("%w: invalid filename", ErrInvalidVideo)
	}
	parts := videoFilenamePattern.FindStringSubmatch(name)
	if parts == nil {
		return Filename{}, fmt.Errorf("%w: filename must match MM_DD_YY_HHMMSS_camera_NNN.mp4", ErrInvalidVideo)
	}
	sequence, err := strconv.Atoi(parts[6])
	if err != nil || sequence < 1 {
		return Filename{}, fmt.Errorf("%w: video sequence must be between 001 and 999", ErrInvalidVideo)
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return Filename{}, fmt.Errorf("%w: invalid recording timezone", ErrInvalidVideo)
	}
	started, err := time.ParseInLocation("01_02_06_150405", parts[1]+"_"+parts[2]+"_"+parts[3]+"_"+parts[4], location)
	if err != nil {
		return Filename{}, fmt.Errorf("%w: invalid date or recording time: %v", ErrInvalidVideo, err)
	}
	return Filename{Camera: parts[5], Sequence: sequence, RecordedAt: started}, nil
}