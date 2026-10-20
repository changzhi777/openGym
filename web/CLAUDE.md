[根目录](../CLAUDE.md) > **web · nginx 容器镜像**

# web/ — web 容器（构建 + 服务 + 反代）

> 3 个文件，职责单一：把 `frontend/` 多阶段构建成静态文件，用 nginx 服务并反代 `/api` → api 容器——**单一源（single origin）是 passkey 的硬要求**。

## 文件清单（全量）

| 文件 | 职责 |
|------|------|
| `Dockerfile` | 多阶段：Node 22 构建 frontend → nginx 镜像（与 api 同发布到 ghcr.io/duartesantos8/opengym-web 与 GitLab registry） |
| `nginx.conf.template` | 启动时按环境变量渲染的 nginx 配置：`NGINX_PORT`(80) / `BACKEND`(api) / `PORT`(3000) / `RESOLVER`(127.0.0.11) / `CF_CONNECTING_IP` / `BASE_PATH`（子路径部署）/ `MEDIA_UPLOAD_MAX`(48m)。**预构建镜像也能改主机/端口，无需重建** |
| `05-check-base-path.sh` | 容器启动早期的 BASE_PATH 一致性检查 |

## 与整体的接线

- compose 里 `web.depends_on: api`，对外端口 `${WEB_PORT:-8080}:${NGINX_PORT:-80}`
- nginx 同时覆盖（overwrite）`X-Forwarded-For`/`X-Real-IP`、丢弃客户端伪造的 `CF-Connecting-IP`——api 的限流/审计依赖"无人可选地址"这一性质，**改代理头透传前先读根 .env.example 的 Activity log 节**
- 动作媒体打进本镜像（v1.4.0 起），由 nginx 直接服务
- 图片缓存 7 天；`no-cache, must-revalidate` 用于 HTML

## 测试

api 侧有 `api/test/k8s-web-port.test.js` 跨模块校验端口约定；镜像构建/启动在 CI（test.yml 构建 api 的两个 target，docker-publish.yml 发布）。

---

变更记录: 2026-10-20 V1.0 初版（init-architect）
