[← 沐禾健身 根 CLAUDE.md](../CLAUDE.md) · **helm · Helm Chart**

# helm/ — Kubernetes Helm Chart（opengym）

> application 型 Chart（chart version 0.2.1 / appVersion "1.4.1"），把 api + web 双容器栈部署到 K8s。与 `kubernetes/`（裸 manifests 示例）二选一，Chart 是参数化版本。本轮核对：无文件变化；Chart 面向**上游镜像**（ghcr.io/duartesantos8/*），fork 的 Go 后端栈（backend-go）尚无 chart/manifest，K8s 路线未启用。

## 文件清单

| 路径 | 职责 |
|------|------|
| `Chart.yaml` | chart 元数据（v2 apiVersion） |
| `values.yaml` | 全部可调参数（镜像 tag、副本、存储、ingress/证书、env 映射） |
| `templates/deployment.yaml` · `service.yaml` · `serviceaccount.yaml` | 工作负载与网络 |
| `templates/configmap.yaml` · `secret.yaml` | 配置与敏感项（对照根 `.env.example` 的 env 合同；fork 部署时 RP_NAME 传"沐禾健身"） |
| `templates/pvc.yaml` | **data 持久卷**（对应 compose 的 `./data` 挂载——实例的整条命脉） |
| `templates/ingress.yaml` · `httproute.yaml` · `certificate.yaml` | 入口两种形态（Ingress / Gateway API HTTPRoute）+ TLS |
| `templates/_helpers.tpl` · `NOTES.txt` | 模板函数 / 安装后提示 |
| `templates/tests/test-connection.yaml` | helm test 钩子 |
| `README.md` | Chart 使用说明 |

## 使用

```bash
helm install opengym ./helm -f my-values.yaml   # 参考 helm/README.md 与 values.yaml 注释
```

配套文档：[docs/SELF_HOSTING_KUBERNETES.md](../docs/SELF_HOSTING_KUBERNETES.md)（裸 manifests 路线见 `../kubernetes/`）。

## 注意

- passkey 绑定主机名：ingress/httproute 的 hostname 必须与 `RP_ID`/`ORIGIN` 一致，TLS 是硬要求
- api 的 `api/test/k8s-web-port.test.js` 锁端口约定，改 chart 端口映射时留意
- fork 现实：生产 = PVE CT 151（`scripts/deploy-ct151.sh`）；若未来上 K8s 跑 Go 栈，需为 backend-go 另写 deployment（其 compose.go.yml 的 pg+redis+api 拓扑可作蓝本）

---

变更记录: 2026-10-27 V1.1 核对式更新（init-architect）:fork 部署现实与 Go 栈空白注记，无文件变化；2026-10-20 V1.0 初版
