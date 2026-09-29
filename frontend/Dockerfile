# Orchestrator Web Console image. Build from the frontend/ directory:
#   docker build -t orchestrator-frontend:<tag> frontend
# nginx serves the bundle under /ui/ and proxies /api/ to BACKEND_URL, the
# orchestrator API Service URL injected at deploy time.
FROM node:22-alpine AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
RUN npm run build

FROM nginx:1.29-alpine
COPY deploy/nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /src/dist /usr/share/nginx/html/ui
EXPOSE 8080
