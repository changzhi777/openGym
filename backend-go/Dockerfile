# 多阶段:静态单二进制;运行层 scratch(零基础镜像,只拷二进制+CA)。
# 本地无 gcr 访问,distroless 拉不到 —— scratch 是 Go 静态编译的正统替代。
FROM golang:1.26-alpine AS build
RUN apk add --no-cache ca-certificates
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/api ./cmd/api

FROM scratch
COPY --from=build /out/api /api
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
EXPOSE 3100
USER 65534:65534
ENTRYPOINT ["/api"]
