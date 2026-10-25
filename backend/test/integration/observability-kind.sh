#!/usr/bin/env bash
# New-user real Kubernetes acceptance; task-owned kind/Compose/volumes only.
set -euo pipefail
umask 077
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
RUN_ID="${OBS_RUN_ID:-obs$(date -u +%m%d%H%M%S)}"
EVIDENCE="${OBS_EVIDENCE:-/tmp/poc-${RUN_ID}}"
[[ "$RUN_ID" =~ ^[a-z0-9]{6,24}$ && "$EVIDENCE" == /tmp/* && ! -e "$EVIDENCE" && "$(realpath -m "$EVIDENCE")" == "$EVIDENCE" ]]
for tool in docker kind kubectl go node jq curl; do command -v "$tool" >/dev/null; done
CLUSTER="logging-${RUN_ID}"; PROJECT="$CLUSTER"; TAG="$RUN_ID"; PRIVATE="$EVIDENCE/private"
mkdir -m 700 "$EVIDENCE" "$PRIVATE"
exec > >(tee -a "$EVIDENCE/runner.log") 2>&1
free_port(){ node -e 'const s=require("node:net").createServer();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})'; }
export BACKEND_PORT="$(free_port)" FRONTEND_PORT="$(free_port)"
export BACKEND_IMAGE="orchestrator-backend:$TAG" FRONTEND_IMAGE="orchestrator-frontend:$TAG"
export KUBECONFIG="$PRIVATE/host.kubeconfig"
COMPOSE=(docker compose --env-file /dev/null --project-name "$PROJECT" -f "$REPO/docker-compose.yml" -f "$REPO/deploy/local/compose.kind.yml" -f "$REPO/deploy/local/compose.git-render-bundle.yml")
kind get clusters 2>/dev/null | rg -qx "$CLUSTER" && exit 2
[[ -z "$(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT")" ]]
jq -n --arg run "$RUN_ID" --arg cluster "$CLUSTER" --arg project "$PROJECT" --arg commit "$(git -C "$REPO" rev-parse HEAD)" --argjson bp "$BACKEND_PORT" --argjson fp "$FRONTEND_PORT" '{runId:$run,cluster:$cluster,composeProject:$project,commitUnderTest:$commit,ports:{backend:$bp,frontend:$fp},source:"real adapters, empty catalog, user UI mutations; no telemetry fixture"}' > "$EVIDENCE/run-manifest.json"
(cd "$REPO";sha256sum backend/examples/observability-app/main.go frontend/test/e2e/observability-kind.mjs backend/test/integration/observability-kind.sh) > "$EVIDENCE/source-sha256.txt"
CREATED=0; STACK=0
NETWORK_PREEXISTED=0; docker network inspect kind >/dev/null 2>&1 && NETWORK_PREEXISTED=1
cleanup(){
 status=$?;set +e
 echo "cleanup scenario_exit=$status"
 if [[ "$status" != 0 ]];then
   for service in backend db-setup vault;do cid="$("${COMPOSE[@]}" ps -aq "$service" 2>/dev/null)";[[ -z "$cid" ]] || docker logs "$cid" > "$PRIVATE/$service-failure.log" 2>&1;done
 fi
 {
 if [[ "$STACK" == 1 ]];then "${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1;fi
 [[ -z "$(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT")" ]] && echo 'compose containers absent' || status=1
 [[ -z "$(docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT")" ]] && echo 'compose volumes absent' || status=1
 if [[ "$CREATED" == 1 ]];then kind delete cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG" >/dev/null 2>&1;fi
 kind get clusters 2>/dev/null | rg -qx "$CLUSTER" && status=1 || echo 'owned kind cluster absent'
 for img in "$BACKEND_IMAGE" "$FRONTEND_IMAGE" "observability-app:$TAG";do docker rmi "$img" >/dev/null 2>&1;done
 if [[ "$NETWORK_PREEXISTED" == 0 ]] && docker network inspect kind >/dev/null 2>&1;then
  if [[ "$(docker network inspect kind -f '{{len .Containers}}')" == 0 ]];then docker network rm kind >/dev/null;else echo 'new network retained because it has other consumers';status=1;fi
 fi
 echo "final_status=$status"
 } | tee "$EVIDENCE/cleanup.txt"
 if rg -q '^final_status=0$' "$EVIDENCE/cleanup.txt";then rm -rf "$PRIVATE";else status=1;fi
 exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT;trap 'exit 143' TERM
{ kind version;kubectl version --client -o json | jq -r '.clientVersion.gitVersion';go version;docker version --format '{{.Server.Version}}'; } > "$EVIDENCE/versions.txt"
CREATED=1
kind create cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG" --image kindest/node:v1.36.1 --wait 240s
kind get kubeconfig --internal --name "$CLUSTER" > "$PRIVATE/internal.kubeconfig"
chmod 600 "$PRIVATE/"*kubeconfig
kubectl --context "kind-$CLUSTER" get nodes -o name
# Kind's kubelet serves a self-signed certificate; this opt-out is confined to
# the disposable test cluster, never installed by the product collector.
curl -fsSL --retry 2 https://github.com/kubernetes-sigs/metrics-server/releases/download/v0.8.1/components.yaml > "$PRIVATE/metrics.yaml"
kubectl --context "kind-$CLUSTER" apply -f "$PRIVATE/metrics.yaml" >/dev/null
kubectl --context "kind-$CLUSTER" -n kube-system patch deployment metrics-server --type=json -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]' >/dev/null
kubectl --context "kind-$CLUSTER" -n kube-system rollout status deployment/metrics-server --timeout=240s
(cd "$REPO/backend";CGO_ENABLED=0 go build -trimpath -o "$PRIVATE/orchestrator" ./cmd/orchestrator;CGO_ENABLED=0 go build -trimpath -o "$PRIVATE/observability-app" ./examples/observability-app)
mkdir -p "$PRIVATE/backend-image" "$PRIVATE/frontend-image" "$PRIVATE/app-image"
cp "$PRIVATE/orchestrator" "$PRIVATE/backend-image/"
if [[ -n "${OBS_BACKEND_RUNTIME_IMAGE:-}" ]];then
 printf 'FROM %s\nCOPY --chmod=0755 orchestrator /usr/local/bin/orchestrator\n' "$OBS_BACKEND_RUNTIME_IMAGE" > "$PRIVATE/backend-image/Dockerfile"
 docker image inspect -f '{{.Id}}' "$OBS_BACKEND_RUNTIME_IMAGE" > "$EVIDENCE/backend-runtime-base.txt"
 docker build --quiet -t "$BACKEND_IMAGE" "$PRIVATE/backend-image" >/dev/null
else docker build --quiet -t "$BACKEND_IMAGE" "$REPO/backend" >/dev/null;fi
docker run --rm --entrypoint /bin/sh "$BACKEND_IMAGE" -c 'test -x /usr/local/bin/orchestrator'
(cd "$REPO/frontend";npm run build > "$EVIDENCE/frontend-build.log" 2>&1)
if [[ -n "${OBS_FRONTEND_RUNTIME_IMAGE:-}" ]];then
 cp -R "$REPO/frontend/dist" "$PRIVATE/frontend-image/dist"
 # Build artifacts are public static files; npm inherited the runner's private
 # umask, so restore read/traverse permission for nginx's non-root workers.
 chmod -R a+rX "$PRIVATE/frontend-image/dist"
 printf 'FROM %s\nCOPY dist /usr/share/nginx/html/ui\n' "$OBS_FRONTEND_RUNTIME_IMAGE" > "$PRIVATE/frontend-image/Dockerfile"
 docker image inspect -f '{{.Id}}' "$OBS_FRONTEND_RUNTIME_IMAGE" > "$EVIDENCE/frontend-runtime-base.txt"
 docker build --quiet -t "$FRONTEND_IMAGE" "$PRIVATE/frontend-image" >/dev/null
else docker build --quiet -t "$FRONTEND_IMAGE" "$REPO/frontend" >/dev/null;fi
cp "$PRIVATE/observability-app" "$PRIVATE/app-image/"
cat > "$PRIVATE/app-image/Dockerfile" <<'DOCKER'
FROM scratch
COPY --chmod=0755 observability-app /app
ENV GOMEMLIMIT=240MiB
USER 65532:65532
ENTRYPOINT ["/app"]
DOCKER
docker build --quiet -t "observability-app:$TAG" "$PRIVATE/app-image" >/dev/null
kind load docker-image "observability-app:$TAG" --name "$CLUSTER" >/dev/null
STACK=1
"${COMPOSE[@]}" up -d --wait --wait-timeout 300 --no-build
BC="$("${COMPOSE[@]}" ps -q backend)"
export OBS_URL="http://127.0.0.1:$FRONTEND_PORT" OBS_RUN_ID="$RUN_ID" OBS_EVIDENCE="$EVIDENCE" OBS_PRIVATE="$PRIVATE" OBS_IMAGE="observability-app:$TAG" OBS_CONTEXT="kind-$CLUSTER" OBS_BACKEND_CONTAINER="$BC" OBS_COMPOSE_PROJECT="$PROJECT"
docker inspect -f '{{.Image}}' "$BC" > "$EVIDENCE/backend-image.txt"
curl -fsS "$OBS_URL/ui/sign-in" >/dev/null
echo "Environment ready $OBS_URL"
node --check "$REPO/frontend/test/e2e/observability-kind.mjs"
DISPLAY="${DISPLAY:-:0}" node "$REPO/frontend/test/e2e/observability-kind.mjs"

# Scan exact run-created credential material before private inputs are removed.
node - "$EVIDENCE" <<'JS'
const fs=require('node:fs'),path=require('node:path'),root=process.argv[2];
const config=fs.readFileSync(path.join(root,'private/internal.kubeconfig'),'utf8');
const needles=[...config.matchAll(/^\s*(?:client-key-data|client-certificate-data|certificate-authority-data|token):\s*(.+)$/gm)].map(m=>m[1].trim().replace(/^['"]|['"]$/g,'')).filter(v=>v.length>=16);
function scan(dir){for(const e of fs.readdirSync(dir,{withFileTypes:true})){if(e.name==='private')continue;const p=path.join(dir,e.name);if(e.isDirectory())scan(p);else{const b=fs.readFileSync(p);if(needles.some(n=>b.includes(Buffer.from(n))))throw new Error('Run credential material in public evidence file '+path.relative(root,p));}}}
scan(root);fs.writeFileSync(path.join(root,'secret-scan.txt'),'PASS: exact run credential material absent from public evidence; UI/API checks separately asserted.\n');
JS
