FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/vinovate .

FROM alpine:3.20
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/vinovate /app/vinovate
RUN mkdir -p /app/private-apks && chown -R app:app /app
USER app
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/app/vinovate"]
