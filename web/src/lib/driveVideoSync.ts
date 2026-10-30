import type { DriveVideo } from "./api";

const roadCamFilenamePattern = /^\d{2}_\d{2}_\d{2}_\d{6}_(frontCenter|rearCenter|sideLeft|sideRight)_\d{3}\.mp4$/;

export function isRoadCamFilename(filename: string): boolean {
  return roadCamFilenamePattern.test(filename);
}

export type DriveVideoSegment = {
  video: DriveVideo;
  startMs: number | null;
  endMs: number | null;
  duration: number | null;
};

export function buildDriveVideoSegments(
  videos: DriveVideo[],
  durations: Record<string, number>,
): DriveVideoSegment[] {
  if (videos.length === 0) return [];
  const baseMs = new Date(videos[0].recorded_at).getTime();
  let offsetMs = 0;
  let offsetsKnown = true;
  return videos.map((video) => {
    const duration = durations[video.id] ?? null;
    const startMs = offsetsKnown ? baseMs + offsetMs : null;
    const endMs = startMs != null && duration != null ? startMs + duration * 1000 : null;
    if (duration == null) offsetsKnown = false;
    else if (offsetsKnown) offsetMs += duration * 1000;
    return { video, startMs, endMs, duration };
  });
}

export function driveVideoSegmentAt(
  segments: DriveVideoSegment[],
  timeMs: number,
): DriveVideoSegment | undefined {
  return segments.find(
    (segment) => segment.startMs != null && segment.endMs != null && timeMs >= segment.startMs && timeMs < segment.endMs,
  );
}