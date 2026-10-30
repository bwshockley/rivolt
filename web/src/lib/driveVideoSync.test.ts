import { describe, expect, it } from "vitest";
import { buildDriveVideoSegments, driveVideoSegmentAt, isRoadCamFilename } from "./driveVideoSync";
import type { DriveVideo } from "./api";

function clip(id: string, sequence: number): DriveVideo {
  return {
    id,
    camera: "frontCenter",
    sequence,
    filename: `10_30_26_142501_frontCenter_${String(sequence).padStart(3, "0")}.mp4`,
    recorded_at: "2026-10-30T21:25:01.000Z",
    size_bytes: 100,
    uploaded_at: "2026-10-31T00:00:00.000Z",
  };
}

describe("drive video synchronization", () => {
  it("accepts only the supported Road Cam filename and camera tokens", () => {
    expect(isRoadCamFilename("10_30_26_073907_frontCenter_001.mp4")).toBe(true);
    expect(isRoadCamFilename("10_30_26_073907_fontCenter_001.mp4")).toBe(false);
    expect(isRoadCamFilename("2097.mp4")).toBe(false);
  });

  it("places later clips after the previous clip duration", () => {
    const segments = buildDriveVideoSegments([clip("one", 1), clip("two", 2)], { one: 60, two: 45 });
    expect(segments.map(({ startMs, endMs }) => [startMs, endMs])).toEqual([
      [Date.parse("2026-10-30T21:25:01.000Z"), Date.parse("2026-10-30T21:26:01.000Z")],
      [Date.parse("2026-10-30T21:26:01.000Z"), Date.parse("2026-10-30T21:26:46.000Z")],
    ]);
    expect(driveVideoSegmentAt(segments, Date.parse("2026-10-30T21:26:10.000Z"))?.video.id).toBe("two");
  });

  it("does not guess offsets past a segment whose duration is not loaded", () => {
    const segments = buildDriveVideoSegments([clip("one", 1), clip("two", 2)], {});
    expect(segments[0].startMs).toBe(Date.parse("2026-10-30T21:25:01.000Z"));
    expect(segments[1].startMs).toBeNull();
    expect(driveVideoSegmentAt(segments, Date.parse("2026-10-30T21:26:10.000Z"))).toBeUndefined();
  });
});