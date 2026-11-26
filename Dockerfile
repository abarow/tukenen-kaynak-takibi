# 1) Arayüz: SvelteKit statik çıktı -> backend/internal/web/dist
FROM node:24-alpine AS web
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN mkdir -p ../backend/internal/web/dist && npm run build

# 2) API: arayüzü gömerek tek ikili dosya üret
FROM golang:1.27-alpine AS api
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=web /src/backend/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/kaynak ./cmd/server

# 3) Çalışma imajı
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=api /out/kaynak /kaynak
EXPOSE 8088
ENTRYPOINT ["/kaynak"]
