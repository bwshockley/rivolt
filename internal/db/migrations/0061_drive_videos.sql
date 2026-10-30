-- 0061_drive_videos.sql — private Road Cam clips attached to a drive.
CREATE TABLE IF NOT EXISTS drive_videos (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    drive_id    UUID NOT NULL REFERENCES drives(id) ON DELETE CASCADE,
    camera      TEXT NOT NULL CHECK (camera IN ('frontCenter', 'rearCenter', 'sideLeft', 'sideRight')),
    sequence    INTEGER NOT NULL CHECK (sequence BETWEEN 1 AND 999),
    filename    TEXT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    size_bytes  BIGINT NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 1073741824),
    path        TEXT NOT NULL UNIQUE,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (drive_id, camera, sequence)
);
CREATE INDEX IF NOT EXISTS drive_videos_user_drive_camera
    ON drive_videos (user_id, drive_id, camera, sequence);

ALTER TABLE drive_videos ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS drive_videos_tenant_isolation ON drive_videos;
CREATE POLICY drive_videos_tenant_isolation ON drive_videos
    USING (user_id = rivolt_current_user_id())
    WITH CHECK (user_id = rivolt_current_user_id());