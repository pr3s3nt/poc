#!/usr/bin/env bash
# Human-paced UC-02/03/04 registration video. Local only: fake-adapter backend, temporary
# JSON state, headed Chromium on a private Xvfb display recorded by ffmpeg.
# No Docker, cluster or cloud and no API fixtures: only the seeded local test
# accounts and catalog exist; every shown step is a UI action.
#
# Requires Xvfb, ffmpeg, ffprobe and xdotool. xdotool may come from PATH or
# ORCH_VIDEO_XDOTOOL; without either, the runner extracts it from the
# distribution package into the work directory (no system install).
#
# Output: ${ORCH_VIDEO_DIR:-/tmp/uc02-04-video-<run id>}/uc02-04-review.mp4 plus
# run.json, marks.json, ffprobe.json and frames/*.png. The directory is
# kept; nothing is committed.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
RUN_ID="uc02-04-video-$(date -u +%Y%m%d%H%M%S)-${RANDOM}"
SCREEN="${ORCH_VIDEO_SCREEN:-1440x900}"
MIN_SECONDS="${ORCH_VIDEO_MIN_SECONDS:-90}"
PIDS=()

# Always write into a directory this run creates, so earlier evidence or any
# other files are never overwritten or removed.
if [[ -n "${ORCH_VIDEO_DIR:-}" ]]; then
  if [[ -e "${ORCH_VIDEO_DIR}" || -L "${ORCH_VIDEO_DIR}" ]]; then
    echo "ORCH_VIDEO_DIR ${ORCH_VIDEO_DIR} already exists; choose a new path" >&2
    exit 1
  fi
  mkdir -m 0700 "${ORCH_VIDEO_DIR}"
  WORK="${ORCH_VIDEO_DIR}"
else
  WORK="$(mktemp -d "${TMPDIR:-/tmp}/${RUN_ID}.XXXXXX")"
fi

cleanup() {
  local status=$?
  for pid in "${PIDS[@]}"; do kill "${pid}" 2>/dev/null || true; done
  for pid in "${PIDS[@]}"; do wait "${pid}" 2>/dev/null || true; done
  rm -f "${WORK}/state.json"
  echo "evidence in ${WORK}"
  echo "run-id=${RUN_ID} status=${status}"
  exit "${status}"
}
trap cleanup EXIT

for tool in Xvfb ffmpeg ffprobe; do command -v "${tool}" >/dev/null || { echo "${tool} is required" >&2; exit 1; }; done
[[ -f "${REPO}/frontend/dist/index.html" ]] || { echo "build the Web Console first: cd frontend && npm run build" >&2; exit 1; }
[[ -d "${REPO}/frontend/node_modules/@playwright/test" ]] || { echo "install frontend dependencies first" >&2; exit 1; }

XDOTOOL="${ORCH_VIDEO_XDOTOOL:-$(command -v xdotool || true)}"
if [[ -z "${XDOTOOL}" ]]; then
  mkdir -p "${WORK}/xdotool"
  (cd "${WORK}/xdotool" && apt-get download xdotool libxdo3 >/dev/null 2>&1)
  for deb in "${WORK}"/xdotool/*.deb; do dpkg-deb -x "${deb}" "${WORK}/xdotool/root"; done
  cat > "${WORK}/xdotool/xdotool" <<EOF
#!/usr/bin/env bash
LD_LIBRARY_PATH="${WORK}/xdotool/root/usr/lib/x86_64-linux-gnu" exec "${WORK}/xdotool/root/usr/bin/xdotool" "\$@"
EOF
  chmod 0755 "${WORK}/xdotool/xdotool"
  XDOTOOL="${WORK}/xdotool/xdotool"
fi
[[ -x "${XDOTOOL}" ]] || { echo "xdotool is required for native address-bar input" >&2; exit 1; }

(cd "${ROOT}" && go build -o "${WORK}/orchestrator" ./cmd/orchestrator)
"${WORK}/orchestrator" -addr 127.0.0.1:0 -addr-file "${WORK}/api-addr" -state "${WORK}/state.json" \
  -adapters fake -profile test -run-id "${RUN_ID}" -ui-dir "${REPO}/frontend/dist" \
  > "${WORK}/orchestrator.log" 2>&1 &
PIDS+=($!)
for _ in $(seq 1 80); do [[ -s "${WORK}/api-addr" ]] && break; sleep 0.25; done
[[ -s "${WORK}/api-addr" ]] || { echo "backend did not start; see ${WORK}/orchestrator.log" >&2; exit 1; }
API="http://$(<"${WORK}/api-addr")"
for _ in $(seq 1 40); do curl -fsS "${API}/api/v1/healthz" >/dev/null 2>&1 && break; sleep 0.25; done
curl -fsS "${API}/api/v1/healthz" >/dev/null

# WSLg owns :0, so pick the first free display from :90.
DISPLAY_NUMBER=""
for candidate in $(seq 90 99); do
  if [[ ! -e "/tmp/.X${candidate}-lock" && ! -e "/tmp/.X11-unix/X${candidate}" ]]; then DISPLAY_NUMBER="${candidate}"; break; fi
done
[[ -n "${DISPLAY_NUMBER}" ]] || { echo "no free X display in :90-:99" >&2; exit 1; }
Xvfb ":${DISPLAY_NUMBER}" -screen 0 "${SCREEN}x24" -nolisten tcp 2> "${WORK}/xvfb.log" &
PIDS+=($!)
# WSLg mounts /tmp/.X11-unix read-only, so Xvfb may listen only on the
# abstract socket; its lock file marks a started server.
for _ in $(seq 1 20); do [[ -e "/tmp/.X${DISPLAY_NUMBER}-lock" ]] && break; sleep 0.5; done
[[ -e "/tmp/.X${DISPLAY_NUMBER}-lock" ]]
sleep 1
kill -0 "${PIDS[-1]}"

DISPLAY=":${DISPLAY_NUMBER}" ORCH_E2E_SCREEN="${SCREEN}" ORCH_E2E_XDOTOOL="${XDOTOOL}" \
  ORCH_E2E_URL="${API}" ORCH_E2E_RUN_ID="${RUN_ID}" ORCH_E2E_EVIDENCE_DIR="${WORK}" \
  node "${REPO}/frontend/test/e2e/uc02-04-video-local.mjs" 2>&1 | tee "${WORK}/playwright.log"

VIDEO="${WORK}/uc02-04-review.mp4"
[[ -s "${VIDEO}" ]] || { echo "video was not written" >&2; exit 1; }
ffprobe -v error -show_entries format=duration,size:stream=codec_name,width,height,avg_frame_rate,nb_frames \
  -of json "${VIDEO}" > "${WORK}/ffprobe.json"
# Decode the whole file: a truncated MP4 fails here even when its header parses.
ffmpeg -hide_banner -v error -xerror -i "${VIDEO}" -f null - 2> "${WORK}/decode.log" \
  || { echo "video does not decode cleanly; see ${WORK}/decode.log" >&2; exit 1; }
[[ ! -s "${WORK}/decode.log" ]] || { echo "video decode reported errors; see ${WORK}/decode.log" >&2; exit 1; }
node -e '
  const [probePath, marksPath, screen, minimum] = process.argv.slice(1);
  const probe = require(probePath);
  const marks = require(marksPath);
  const [width, height] = screen.split("x").map(Number);
  const stream = probe.streams?.[0] ?? {};
  const duration = Number(probe.format?.duration);
  const failures = [];
  if (stream.codec_name !== "h264") failures.push(`codec ${stream.codec_name}`);
  if (stream.width !== width || stream.height !== height) failures.push(`resolution ${stream.width}x${stream.height}`);
  if (!(duration >= Number(minimum))) failures.push(`duration ${duration}s < ${minimum}s`);
  if (marks.length < 13) failures.push(`only ${marks.length} marks recorded`);
  const last = marks[marks.length - 1];
  if (!last || last.label !== "signed-out" || last.seconds + 1.5 > duration) failures.push("recording ends before the signed-out mark");
  if (failures.length) { console.error(`invalid video: ${failures.join("; ")}`); process.exit(1); }
' "${WORK}/ffprobe.json" "${WORK}/marks.json" "${SCREEN}" "${MIN_SECONDS}"
DURATION="$(ffprobe -v error -show_entries format=duration -of default=nw=1:nk=1 "${VIDEO}")"

# One frame per recorded mark, 1.5 s after the mark so the screen has settled.
mkdir -p "${WORK}/frames"
node -e '
  const marks = require(process.argv[1]);
  marks.forEach((m, i) => console.log(`${String(i + 1).padStart(2, "0")}-${m.label} ${(m.seconds + 1.5).toFixed(1)}`));
' "${WORK}/marks.json" | while read -r name at; do
  ffmpeg -hide_banner -loglevel error -n -ss "${at}" -i "${VIDEO}" -frames:v 1 "${WORK}/frames/${name}.png" < /dev/null
done
# A blank frame (white page, bare Xvfb root) has almost no luma range.
for frame in "${WORK}"/frames/*.png; do
  range="$(ffprobe -v error -f lavfi -i "movie=${frame},signalstats" \
    -show_entries frame_tags=lavfi.signalstats.YMIN,lavfi.signalstats.YMAX -of default=nw=1:nk=1 | paste -sd' ' | awk '{print $2 - $1}')"
  (( range > 80 )) || { echo "frame ${frame} looks blank (luma range ${range})" >&2; exit 1; }
done
echo "video=${VIDEO} duration=${DURATION}s frames=$(ls "${WORK}/frames" | wc -l)"
