#!/usr/bin/env bash
# Builds one captioned review video from two honestly separate recording
# sessions of the same scenario, with title cards that say so:
#   part 1: first <cut> seconds of a recording (registration of the
#           Connection and Secret Store through the UI),
#   part 2: a complete successful session that reuses those records.
# Captions are rendered from SRT through ASS with explicit resolution into a
# strip below the 1440x900 browser area; the UI is never covered.
#   k8s4f-dev-compose-video.sh <part1-dir> <cut-seconds> <part2-dir> <out-dir>
# Each part dir holds k8s4f-dev-raw.mp4 and captions.srt (see k8s4f-dev-playwright.sh).
set -euo pipefail
P1="${1:?part1 dir}" CUT="${2:?cut seconds}" P2="${3:?part2 dir}" OUT="${4:?out dir}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPO="$(cd "${ROOT}/.." && pwd)"
W=1440 H=900 STRIP=96
CH=$((H + STRIP))
FONT=/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf
mkdir -p "${OUT}"

caption_part() { # <part-dir> <ass-out> <video-out> [extra ffmpeg input args]
  local dir="$1" ass="$2" video="$3"; shift 3
  node "${REPO}/frontend/test/e2e/render-captions.mjs" "${dir}/captions.srt" "${ass}" "${W}" "${CH}"
  ffmpeg -hide_banner -loglevel error -y "$@" -i "${dir}/k8s4f-dev-raw.mp4" \
    -vf "pad=${W}:${CH}:0:0:black,ass=${ass}" -r 15 -c:v libx264 -preset veryfast -crf 24 -pix_fmt yuv420p "${video}"
}
card() { # <textfile> <video-out> <seconds>: one centered drawtext per line (no newline glyphs)
  local filters="" line index=0 count
  count="$(wc -l < "$1")"
  while IFS= read -r line; do
    if [[ -n "${line}" ]]; then
      printf '%s' "${line}" > "${OUT}/.card-line-${index}.txt"
      filters+="drawtext=fontfile=${FONT}:textfile=${OUT}/.card-line-${index}.txt:fontcolor=white:fontsize=28:x=(w-text_w)/2:y=$((CH / 2 - count * 24 + index * 48)),"
    fi
    index=$((index + 1))
  done < "$1"
  ffmpeg -hide_banner -loglevel error -y -f lavfi -i "color=c=0x0b1b2e:s=${W}x${CH}:r=15:d=$3" \
    -vf "${filters%,}" -c:v libx264 -preset veryfast -crf 24 -pix_fmt yuv420p "$2"
  rm -f "${OUT}"/.card-line-*.txt
}
cat > "${OUT}/card-intro.txt" <<'TXT'
Triển khai ứng dụng mẫu lên cụm kind mới k8s-4f qua Web Console

Video ghép từ HAI phiên ghi hình riêng biệt, không phải một lần chạy liền mạch:
Phần 1: đăng ký Connection và Secret store bằng giao diện
Phần 2: tạo Application, Deploy và kiểm tra (dùng lại hai bản ghi từ Phần 1)
TXT
cat > "${OUT}/card-part2.txt" <<'TXT'
Phần 2 — phiên ghi hình thứ hai (đã thành công đầy đủ)

Connection "K8S-4F" và Secret store "K8S-4F Vault" đã được đăng ký ở Phần 1,
nên phiên này chỉ kiểm tra và dùng lại chúng, rồi tạo Application mới.
TXT
caption_part "${P1}" "${OUT}/part1.ass" "${OUT}/part1.mp4" -t "${CUT}"
caption_part "${P2}" "${OUT}/part2.ass" "${OUT}/part2.mp4"
card "${OUT}/card-intro.txt" "${OUT}/card-intro.mp4" 8
card "${OUT}/card-part2.txt" "${OUT}/card-part2.mp4" 7
ffmpeg -hide_banner -loglevel error -y -i "${OUT}/card-intro.mp4" -i "${OUT}/part1.mp4" -i "${OUT}/card-part2.mp4" -i "${OUT}/part2.mp4" \
  -filter_complex "[0:v][1:v][2:v][3:v]concat=n=4:v=1:a=0[v]" -map "[v]" -r 15 -c:v libx264 -preset veryfast -crf 24 -pix_fmt yuv420p -movflags +faststart "${OUT}/k8s4f-dev-full.mp4"
ffprobe -v error -show_entries format=duration,size:stream=codec_name,width,height,avg_frame_rate,nb_frames -of json "${OUT}/k8s4f-dev-full.mp4" > "${OUT}/k8s4f-dev-full-ffprobe.json"
ffmpeg -hide_banner -v error -xerror -i "${OUT}/k8s4f-dev-full.mp4" -f null - 2> "${OUT}/k8s4f-dev-full-decode.log"
[[ ! -s "${OUT}/k8s4f-dev-full-decode.log" ]]
echo "full=${OUT}/k8s4f-dev-full.mp4 duration=$(jq -r .format.duration "${OUT}/k8s4f-dev-full-ffprobe.json")"
