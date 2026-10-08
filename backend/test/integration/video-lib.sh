# Shared helpers for full-window review recordings: a private Xvfb display,
# xdotool for native browser input and ffprobe/ffmpeg checks of the MP4.
# Source from a runner that sets WORK and keeps its background PIDs in PIDS.
# Nothing here touches Kubernetes, cloud accounts or files outside WORK.

# video_require_tools: Xvfb, ffmpeg and ffprobe must be on PATH.
video_require_tools() {
  local tool
  for tool in Xvfb ffmpeg ffprobe; do command -v "${tool}" >/dev/null || { echo "${tool} is required" >&2; return 1; }; done
}

# video_require_fresh_dist <repo>: the Web Console bundle must exist and be
# newer than every frontend source file, so the video shows current UI code.
video_require_fresh_dist() {
  local repo="$1"
  [[ -f "${repo}/frontend/dist/index.html" ]] || { echo "build the Web Console first: cd frontend && npm run build" >&2; return 1; }
  if [[ -n "$(find "${repo}/frontend/src" "${repo}/frontend/index.html" -newer "${repo}/frontend/dist/index.html" -print -quit)" ]]; then
    echo "frontend/dist is older than frontend/src; rebuild: cd frontend && npm run build" >&2
    return 1
  fi
  [[ -d "${repo}/frontend/node_modules/@playwright/test" ]] || { echo "install frontend dependencies first" >&2; return 1; }
}

# video_xdotool: prints an xdotool path from ORCH_VIDEO_XDOTOOL or PATH, or
# extracts the distribution package into WORK (no system install).
video_xdotool() {
  local tool="${ORCH_VIDEO_XDOTOOL:-$(command -v xdotool || true)}"
  if [[ -z "${tool}" ]]; then
    mkdir -p "${WORK}/xdotool"
    (cd "${WORK}/xdotool" && apt-get download xdotool libxdo3 >/dev/null 2>&1)
    local deb
    for deb in "${WORK}"/xdotool/*.deb; do dpkg-deb -x "${deb}" "${WORK}/xdotool/root"; done
    cat > "${WORK}/xdotool/xdotool" <<EOF
#!/usr/bin/env bash
LD_LIBRARY_PATH="${WORK}/xdotool/root/usr/lib/x86_64-linux-gnu" exec "${WORK}/xdotool/root/usr/bin/xdotool" "\$@"
EOF
    chmod 0755 "${WORK}/xdotool/xdotool"
    tool="${WORK}/xdotool/xdotool"
  fi
  [[ -x "${tool}" ]] || { echo "xdotool is required for native browser input" >&2; return 1; }
  printf '%s\n' "${tool}"
}

# video_start_xvfb <screen>: starts Xvfb on the first free display from :90
# (WSLg owns :0), appends its PID to PIDS and sets VIDEO_DISPLAY.
video_start_xvfb() {
  local screen="$1" candidate
  VIDEO_DISPLAY=""
  for candidate in $(seq 90 99); do
    if [[ ! -e "/tmp/.X${candidate}-lock" && ! -e "/tmp/.X11-unix/X${candidate}" ]]; then VIDEO_DISPLAY=":${candidate}"; break; fi
  done
  [[ -n "${VIDEO_DISPLAY}" ]] || { echo "no free X display in :90-:99" >&2; return 1; }
  Xvfb "${VIDEO_DISPLAY}" -screen 0 "${screen}x24" -nolisten tcp 2> "${WORK}/xvfb.log" &
  PIDS+=($!)
  # WSLg mounts /tmp/.X11-unix read-only, so Xvfb may listen only on the
  # abstract socket; its lock file marks a started server.
  local _
  for _ in $(seq 1 20); do [[ -e "/tmp/.X${VIDEO_DISPLAY#:}-lock" ]] && break; sleep 0.5; done
  [[ -e "/tmp/.X${VIDEO_DISPLAY#:}-lock" ]] || { echo "Xvfb did not start; see ${WORK}/xvfb.log" >&2; return 1; }
  sleep 1
  kill -0 "${PIDS[-1]}"
}

# video_validate <video> <screen> <min-seconds> <min-marks> <last-mark>:
# probes codec/size/duration, decodes the whole file, extracts one frame per
# mark into WORK/frames and rejects blank frames. Prints a summary line.
video_validate() {
  local video="$1" screen="$2" minimum="$3" min_marks="$4" last="$5"
  [[ -s "${video}" ]] || { echo "video was not written" >&2; return 1; }
  ffprobe -v error -show_entries format=duration,size:stream=codec_name,width,height,avg_frame_rate,nb_frames \
    -of json "${video}" > "${WORK}/ffprobe.json"
  # Decode the whole file: a truncated MP4 fails here even when its header parses.
  ffmpeg -hide_banner -v error -xerror -i "${video}" -f null - 2> "${WORK}/decode.log" \
    || { echo "video does not decode cleanly; see ${WORK}/decode.log" >&2; return 1; }
  [[ ! -s "${WORK}/decode.log" ]] || { echo "video decode reported errors; see ${WORK}/decode.log" >&2; return 1; }
  node -e '
    const [probePath, marksPath, screen, minimum, minMarks, lastLabel] = process.argv.slice(1);
    const probe = require(probePath);
    const marks = require(marksPath);
    const [width, height] = screen.split("x").map(Number);
    const stream = probe.streams?.[0] ?? {};
    const duration = Number(probe.format?.duration);
    const failures = [];
    if (stream.codec_name !== "h264") failures.push(`codec ${stream.codec_name}`);
    if (stream.width !== width || stream.height !== height) failures.push(`resolution ${stream.width}x${stream.height}`);
    if (!(duration >= Number(minimum))) failures.push(`duration ${duration}s < ${minimum}s`);
    if (marks.length < Number(minMarks)) failures.push(`only ${marks.length} marks recorded`);
    const last = marks[marks.length - 1];
    if (!last || last.label !== lastLabel || last.seconds + 1.5 > duration) failures.push(`recording ends before the ${lastLabel} mark`);
    if (failures.length) { console.error(`invalid video: ${failures.join("; ")}`); process.exit(1); }
  ' "${WORK}/ffprobe.json" "${WORK}/marks.json" "${screen}" "${minimum}" "${min_marks}" "${last}"
  local duration name at frame range
  duration="$(ffprobe -v error -show_entries format=duration -of default=nw=1:nk=1 "${video}")"
  # One frame per recorded mark, 1.5 s after the mark so the screen has settled.
  mkdir -p "${WORK}/frames"
  while read -r name at; do
    ffmpeg -hide_banner -loglevel error -n -ss "${at}" -i "${video}" -frames:v 1 "${WORK}/frames/${name}.png" < /dev/null
  done < <(node -e '
    const marks = require(process.argv[1]);
    marks.forEach((m, i) => console.log(`${String(i + 1).padStart(2, "0")}-${m.label} ${(m.seconds + 1.5).toFixed(1)}`));
  ' "${WORK}/marks.json")
  # A blank frame (white page, bare Xvfb root) has almost no luma range.
  for frame in "${WORK}"/frames/*.png; do
    range="$(ffprobe -v error -f lavfi -i "movie=${frame},signalstats" \
      -show_entries frame_tags=lavfi.signalstats.YMIN,lavfi.signalstats.YMAX -of default=nw=1:nk=1 | paste -sd' ' | awk '{print $2 - $1}')"
    (( range > 80 )) || { echo "frame ${frame} looks blank (luma range ${range})" >&2; return 1; }
  done
  echo "video=${video} duration=${duration}s frames=$(find "${WORK}/frames" -name '*.png' | wc -l)"
}

# video_require_secret_store <api-url> <store-key>: proves, as the seeded local
# Developer, that the Organization offers the named READY Secret Store and
# exports it as ORCH_E2E_SECRET_STORE for the browser flow. Flows that write a
# Secret into a new Environment must select a store through the UI; the product
# never falls back to one. The store checked here is the explicit platform
# store the backend seeds from its Vault flags (key platform-vault); it is
# separate from the Connection credential store (-connection-vault-*).
# The password is the fixed local/test account; nothing secret is printed.
video_require_secret_store() {
  local api="$1" key="$2" jar choices
  jar="$(mktemp)"
  choices="$(mktemp)"
  if ! curl --connect-timeout 5 --max-time 30 -fsS -c "${jar}" -H 'Content-Type: application/json' \
      -d '{"username":"developer","password":"test-password"}' "${api}/api/v1/auth/sign-in" >/dev/null \
    || ! curl --connect-timeout 5 --max-time 30 -fsS -b "${jar}" "${api}/api/v1/secret-store-choices" > "${choices}" \
    || ! jq -e --arg key "${key}" '.secretStores | any(.key == $key and .status == "READY")' "${choices}" >/dev/null; then
    rm -f "${jar}" "${choices}"
    echo "secret store ${key} is not offered READY by ${api}; start the backend with -vault-address/-vault-token-file so it seeds the platform store" >&2
    return 1
  fi
  rm -f "${jar}" "${choices}"
  export ORCH_E2E_SECRET_STORE="${key}"
  echo "secret store ${key} is READY and will be selected through the UI"
}
