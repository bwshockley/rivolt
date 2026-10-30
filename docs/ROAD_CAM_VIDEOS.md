# Road Cam videos

Rivolt stores uploaded Road Cam clips on disk and keeps a small manifest in
Postgres. The drive page's Video section uploads one MP4 at a time, groups
clips by camera and sequence, and serves them through authenticated,
range-enabled requests so browser seeking works. Video bytes are not included
in the JSON backup or CSV export. For a full restore, back up both Postgres
and the video directory.

## Storage on Synology

The Docker image runs as the non-root user with UID/GID `65532:65532`. By
default, video files are created under `DATA_DIR/videos` (normally
`/data/videos`). For a separate DSM volume, create a shared folder such as
`/volume1/docker/rivolt/videos`, then bind-mount it into the Rivolt service:

```yaml
services:
  rivolt:
    volumes:
      - rivolt-data:/data
      - /volume1/docker/rivolt/videos:/data/videos:rw
```

Create the host directory before starting the container. Grant UID/GID
`65532:65532` read, write, and directory-traverse permissions on that folder.
On a filesystem managed with DSM ACLs, grant those permissions to the
container identity (or a dedicated group containing it) and do not grant
access to `Everyone` or unrelated users. If using POSIX ownership, set the
folder owner to `65532:65532` and permissions to `0700`. Do not use `0777`.
Rivolt creates per-user/per-drive subdirectories named with the drive's local
start time and internal drive ID, using `0700` permissions; video files are
created with `0600` permissions.

Keep the folder private: uploads and playback use the signed-in API and are
not exposed as static files. Ensure any reverse proxy in front of Rivolt
allows request bodies up to 1 GiB plus multipart overhead and has a timeout
long enough for a large upload. Rivolt allows up to two hours for the upload
and video stream requests. A proxy/CDN with a smaller body-size limit must be
configured to bypass or raise that limit for Rivolt.

The default per-user quota is 100 GiB. Set `RIVOLT_VIDEO_QUOTA_GIB` in the
container environment to change it; `0` disables the quota. Every individual
file is still limited to 1 GiB. Monitor free space on the NAS and include the
video directory in your regular backup plan.

## Filename and synchronization

Accepted filenames use this exact form:

```text
MM_DD_YY_HHMMSS_camera_NNN.mp4
```

The camera is `frontCenter`, `rearCenter`, `sideLeft`, or `sideRight`; the
sequence is `001` through `999`. The timestamp is interpreted in the selected
display time zone (Settings → Display → Time zone; `auto` uses the browser's
local zone). Upload from a computer whose local zone matches the vehicle's
recording zone, or select that zone in Rivolt before uploading. The timestamp
is converted to an absolute instant and used as the clip-stream start. Browser
MP4 metadata supplies each segment's duration, so later segments continue
from the previous segment. If the upload computer uses the wrong time zone,
the video will still play, but its cursor will be offset from the drive.

The video cursor is shared with Drive timeline and Route. Playback updates
both; moving the cursor from either existing view seeks the selected camera.
Changing camera keeps the current drive timestamp and seeks to that position
in the new camera's segments when covered.