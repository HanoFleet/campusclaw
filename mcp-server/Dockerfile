FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/mcp-server .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /out/mcp-server /mcp-server
EXPOSE 8090
ENTRYPOINT ["/mcp-server"]
