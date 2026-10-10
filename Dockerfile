FROM golang:1.22-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
RUN GOTOOLCHAIN=local GOFLAGS=-mod=vendor GOTELEMETRY=off go build -o /out/aml-api ./cmd/aml-api
RUN GOTOOLCHAIN=local GOFLAGS=-mod=vendor GOTELEMETRY=off go build -o /out/aml-worker ./cmd/aml-worker
RUN GOTOOLCHAIN=local GOFLAGS=-mod=vendor GOTELEMETRY=off go build -o /out/aml-migrate ./cmd/aml-migrate

FROM alpine:3.20 AS runtime
WORKDIR /app
COPY --from=go-build /out/aml-api /app/aml-api
COPY --from=go-build /out/aml-worker /app/aml-worker
COPY --from=go-build /out/aml-migrate /app/aml-migrate
COPY --from=go-build /src/migrations /app/migrations
EXPOSE 8080
CMD ["/app/aml-api", "-addr", ":8080"]

FROM nginx:1.27-alpine AS ui
COPY web/dist /usr/share/nginx/html
RUN printf '%s\n' 'window.__SENTINEL_CONFIG__ = { apiBaseUrl: "" };' > /usr/share/nginx/html/runtime-config.js     && printf '%s\n'       'server {'       '  listen 80;'       '  server_name _;'       '  root /usr/share/nginx/html;'       '  index index.html;'       '  location /api/ { proxy_pass http://aml-api:8080; }'       '  location /healthz { proxy_pass http://aml-api:8080; }'       '  location /readyz { proxy_pass http://aml-api:8080; }'       '  location / { try_files $uri $uri/ /index.html; }'       '}' > /etc/nginx/conf.d/default.conf
