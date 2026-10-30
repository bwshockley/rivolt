import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { backend, type Drive, type DriveVideo } from "../lib/api";
import { buildDriveVideoSegments, driveVideoSegmentAt } from "../lib/driveVideoSync";
import { displayTimeZone } from "../lib/format";
import { Card } from "./ui";

const cameras: { id: DriveVideo["camera"]; label: string }[] = [
  { id: "frontCenter", label: "Front" },
  { id: "rearCenter", label: "Rear" },
  { id: "sideLeft", label: "Left" },
  { id: "sideRight", label: "Right" },
];
const noVideos: DriveVideo[] = [];

export function DriveVideoPanel({
  drive,
  cursorMs,
  onCursorChange,
}: {
  drive: Drive;
  cursorMs: number | null;
  onCursorChange: (time: number | null) => void;
}) {
  const queryClient = useQueryClient();
  const videos = useQuery({
    queryKey: ["drive-videos", drive.ID],
    queryFn: () => backend.driveVideos(drive.ID),
  });
  const [camera, setCamera] = useState<DriveVideo["camera"]>("frontCenter");
  const [activeID, setActiveID] = useState<string | null>(null);
  const [durations, setDurations] = useState<Record<string, number>>({});
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState("");
  const [error, setError] = useState("");
  const fileInput = useRef<HTMLInputElement>(null);
  const player = useRef<HTMLVideoElement>(null);
  const pendingSeekMs = useRef<number | null>(null);
  const videoCursorMs = useRef<number | null>(null);
  const autoplayNext = useRef(false);
  const initializedDrive = useRef("");

  const allVideos = videos.data ?? noVideos;
  const selectedVideos = useMemo(
    () => allVideos.filter((video) => video.camera === camera).sort((a, b) => a.sequence - b.sequence),
    [allVideos, camera],
  );
  const segments = useMemo(
    () => buildDriveVideoSegments(selectedVideos, durations),
    [selectedVideos, durations],
  );
  const activeSegment =
    segments.find((segment) => segment.video.id === activeID) ??
    (activeID == null && cursorMs == null ? segments[0] : undefined);

  useEffect(() => {
    if (initializedDrive.current !== drive.ID) {
      initializedDrive.current = drive.ID;
      setCamera("frontCenter");
      setActiveID(null);
      setDurations({});
      pendingSeekMs.current = null;
      videoCursorMs.current = null;
      autoplayNext.current = false;
    }
  }, [drive.ID]);

  useEffect(() => {
    if (cursorMs == null && selectedVideos.length > 0 && segments[0]?.startMs != null) {
      if (initializedDrive.current === drive.ID && videoCursorMs.current == null) {
        videoCursorMs.current = segments[0].startMs;
        setActiveID(segments[0].video.id);
        onCursorChange(segments[0].startMs);
      }
      return;
    }
    if (cursorMs == null) return;
    if (videoCursorMs.current != null && Math.abs(cursorMs - videoCursorMs.current) < 50) {
      videoCursorMs.current = cursorMs;
      return;
    }
    videoCursorMs.current = null;
    const target = driveVideoSegmentAt(segments, cursorMs);
    if (!target) return;
    pendingSeekMs.current = cursorMs;
    if (target.video.id !== activeSegment?.video.id) {
      setActiveID(target.video.id);
      return;
    }
    const element = player.current;
    if (element && element.readyState >= 1 && target.startMs != null) {
      const seconds = Math.max(0, (cursorMs - target.startMs) / 1000);
      if (Math.abs(element.currentTime - seconds) > 0.75) element.currentTime = seconds;
    }
  }, [activeSegment?.video.id, cursorMs, drive.ID, onCursorChange, segments, selectedVideos.length]);

  function rememberDuration(videoID: string, duration: number) {
    if (Number.isFinite(duration) && duration > 0) {
      setDurations((current) => current[videoID] === duration ? current : { ...current, [videoID]: duration });
    }
  }

  function syncFromPlayer() {
    const element = player.current;
    if (!element || activeSegment?.startMs == null) return;
    const time = activeSegment.startMs + element.currentTime * 1000;
    videoCursorMs.current = time;
    onCursorChange(time);
  }

  function changeCamera(next: DriveVideo["camera"]) {
    videoCursorMs.current = null;
    pendingSeekMs.current = cursorMs;
    setActiveID(null);
    setCamera(next);
  }

  async function uploadFiles(files: FileList | null) {
    if (!files?.length) return;
    const selected = Array.from(files);
    setBusy(true);
    setError("");
    const timezone = displayTimeZone() || Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
    try {
      for (let index = 0; index < selected.length; index++) {
        const file = selected[index];
        setProgress(`Uploading ${index + 1} of ${selected.length}: ${file.name}`);
        const form = new FormData();
        form.append("timezone", timezone);
        form.append("file", file, file.name);
        await backend.uploadDriveVideo(drive.ID, form);
      }
      await queryClient.invalidateQueries({ queryKey: ["drive-videos", drive.ID] });
      setProgress(`Uploaded ${selected.length} video${selected.length === 1 ? "" : "s"}.`);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Video upload failed.");
      await queryClient.invalidateQueries({ queryKey: ["drive-videos", drive.ID] });
    } finally {
      setBusy(false);
      if (fileInput.current) fileInput.current.value = "";
    }
  }

  async function deleteVideo(video: DriveVideo) {
    setBusy(true);
    setError("");
    try {
      await backend.deleteDriveVideo(drive.ID, video.id);
      await queryClient.invalidateQueries({ queryKey: ["drive-videos", drive.ID] });
      setDurations((current) => {
        const next = { ...current };
        delete next[video.id];
        return next;
      });
      if (activeID === video.id) setActiveID(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not delete video.");
    } finally {
      setBusy(false);
    }
  }

  async function deleteAll() {
    if (!window.confirm(`Delete all ${allVideos.length} videos for this drive? This cannot be undone.`)) return;
    setBusy(true);
    setError("");
    try {
      for (const video of allVideos) await backend.deleteDriveVideo(drive.ID, video.id);
      setActiveID(null);
      setDurations({});
      await queryClient.invalidateQueries({ queryKey: ["drive-videos", drive.ID] });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not delete all videos.");
      await queryClient.invalidateQueries({ queryKey: ["drive-videos", drive.ID] });
    } finally {
      setBusy(false);
    }
  }

  const cameraCount = (id: DriveVideo["camera"]) => allVideos.filter((video) => video.camera === id).length;

  return (
    <Card
      title="Video"
      actions={
        <div className="flex items-center gap-2">
          <input
            ref={fileInput}
            type="file"
            accept="video/mp4,.mp4"
            multiple
            className="hidden"
            onChange={(event) => void uploadFiles(event.currentTarget.files)}
          />
          <button
            type="button"
            disabled={busy}
            onClick={() => fileInput.current?.click()}
            className="rounded-md border border-neutral-700 px-2.5 py-1 text-xs text-neutral-200 hover:bg-neutral-800 disabled:opacity-50"
            title="Upload Road Cam MP4 files for this drive"
          >
            Upload videos
          </button>
          {allVideos.length > 0 && (
            <button
              type="button"
              disabled={busy}
              onClick={() => void deleteAll()}
              className="rounded-md border border-rose-900/70 px-2.5 py-1 text-xs text-rose-300 hover:bg-rose-950/50 disabled:opacity-50"
              title="Delete all videos for this drive"
            >
              Delete all
            </button>
          )}
        </div>
      }
    >
      {videos.isLoading ? (
        <p className="text-sm text-neutral-400">Loading videos…</p>
      ) : videos.isError ? (
        <p className="text-sm text-rose-300">Could not load videos: {String(videos.error)}</p>
      ) : (
        <>
          <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
            <div role="group" aria-label="Camera" className="inline-flex rounded-md border border-neutral-700 p-0.5">
              {cameras.map((item) => (
                <button
                  key={item.id}
                  type="button"
                  aria-pressed={camera === item.id}
                  aria-label={`${item.id} camera`}
                  onClick={() => changeCamera(item.id)}
                  className={`rounded px-3 py-1.5 text-xs ${camera === item.id ? "bg-emerald-700 text-white" : "text-neutral-400 hover:text-neutral-100"}`}
                >
                  {item.label}{cameraCount(item.id) > 0 ? ` (${cameraCount(item.id)})` : ""}
                </button>
              ))}
            </div>
            {selectedVideos.length > 0 && (
              <span className="text-xs text-neutral-500">
                {activeSegment ? `${activeSegment.video.filename}` : `${selectedVideos.length} clips`}
              </span>
            )}
          </div>

          {activeSegment ? (
            <video
              key={activeSegment.video.id}
              ref={player}
              src={backend.driveVideoURL(drive.ID, activeSegment.video.id)}
              controls
              playsInline
              preload="metadata"
              className="aspect-video w-full rounded-md bg-black object-contain"
              onLoadedMetadata={(event) => {
                rememberDuration(activeSegment.video.id, event.currentTarget.duration);
                const pending = pendingSeekMs.current;
                if (pending != null && activeSegment.startMs != null) {
                  event.currentTarget.currentTime = Math.max(0, (pending - activeSegment.startMs) / 1000);
                  pendingSeekMs.current = null;
                }
                if (autoplayNext.current) {
                  autoplayNext.current = false;
                  void event.currentTarget.play().catch(() => undefined);
                }
              }}
              onTimeUpdate={syncFromPlayer}
              onSeeked={syncFromPlayer}
              onEnded={() => {
                const index = segments.findIndex((segment) => segment.video.id === activeSegment.video.id);
                const next = segments[index + 1];
                if (!next) return;
                const nextTime = next.startMs ?? activeSegment.endMs;
                if (nextTime != null) {
                  pendingSeekMs.current = nextTime;
                  videoCursorMs.current = nextTime;
                  onCursorChange(nextTime);
                }
                autoplayNext.current = true;
                setActiveID(next.video.id);
              }}
            />
          ) : (
            <div className="flex aspect-video w-full items-center justify-center rounded-md border border-neutral-800 bg-black/70 px-4 text-center text-sm text-neutral-400">
              {allVideos.length === 0
                ? "No Road Cam videos uploaded for this drive."
                : `No ${cameras.find((item) => item.id === camera)?.label.toLowerCase()} camera videos uploaded for this drive.`}
            </div>
          )}

          {selectedVideos.map((video) => (
            <video
              key={`metadata-${video.id}`}
              src={backend.driveVideoURL(drive.ID, video.id)}
              preload="metadata"
              muted
              aria-hidden="true"
              className="hidden"
              onLoadedMetadata={(event) => rememberDuration(video.id, event.currentTarget.duration)}
            />
          ))}

          {selectedVideos.length > 0 && (
            <ul className="mt-3 divide-y divide-neutral-800 border-t border-neutral-800">
              {selectedVideos.map((video) => (
                <li key={video.id} className="flex items-center justify-between gap-3 py-2 text-xs">
                  <span className="min-w-0 truncate text-neutral-400">
                    {String(video.sequence).padStart(3, "0")} · {video.filename}
                  </span>
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => void deleteVideo(video)}
                    className="shrink-0 text-rose-300 hover:text-rose-200 disabled:opacity-50"
                    aria-label={`Delete ${video.filename}`}
                    title={`Delete ${video.filename}`}
                  >
                    Delete
                  </button>
                </li>
              ))}
            </ul>
          )}
        </>
      )}
      {busy && progress && <p className="mt-3 text-xs text-neutral-400" role="status">{progress}</p>}
      {!busy && progress && <p className="mt-3 text-xs text-emerald-300" role="status">{progress}</p>}
      {error && <p className="mt-3 text-xs text-rose-300" role="alert">{error}</p>}
    </Card>
  );
}