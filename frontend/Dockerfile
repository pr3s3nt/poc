# Orchestrator Web Console image. Build from the frontend/ directory:
#   docker build -t orchestrator-frontend:<tag> frontend
# nginx serves the bundle under /ui/ and proxies /api/ to BACKEND_URL, the
# orchestrator API Service URL injected at deploy time.
FROM mirror.gcr.io/library/node:22-alpine AS build
ENV NODE_TLS_REJECT_UNAUTHORIZED=0
ENV npm_config_strict_ssl=false
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
RUN npm run build

FROM mirror.gcr.io/library/nginx:1-alpine
COPY deploy/nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /src/dist /usr/share/nginx/html/ui
EXPOSE 8080
