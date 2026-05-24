# Multica 二次开发环境交接文档

## 已完成的工作

### 1. 服务器部署 (49.232.233.9)

| 服务 | 端口 | 运行方式 | 说明 |
|------|------|----------|------|
| **Nginx** | 80 | systemd | 反代前端+后端+WebSocket |
| **Next.js 前端** | 3000 | node standalone | 生产构建，Nginx 反代 |
| **Go 后端** | 8080 | 二进制直接运行 | 从本地交叉编译上传 |
| **PostgreSQL 17** | 5432 | Docker (host网络) | pgvector 支持，数据持久化 |
| **Multica Daemon** | - | 后台进程 | 检测到 opencode agent |
| **OpenCode v1.15.10** | - | 二进制 | 连接 doubao-seed 模型 |

### 2. 服务器账号信息

```
SSH: ubuntu@49.232.233.9
密码: weBright11ZZ

PostgreSQL:
  用户: multica
  密码: multica
  数据库: multica
  连接串: postgres://multica:multica@49.232.233.9:5432/multica?sslmode=disable

后端 JWT Secret: 316c6d994f7c0e1d0308893fc645d7064d3b874e9a212647b74db2730de45909
验证码(开发): 888888

Multica 登录:
  邮箱: 2406753447@qq.com
  验证码: 888888
```

### 3. AI Agent 配置

```
模型: 字节 doubao-seed-2-0-lite
Endpoint: https://ark.cn-beijing.volces.com/api/v3
EP ID: ep-20260514110933-mzh58
ARK_API_KEY: ark-3a6a7711-6bc2-444d-9572-f9e1887a7aab-40a6c
```

OpenCode 配置文件: `~/.opencode.json` (服务器上)

### 4. 本地开发环境 (macOS)

已安装:
- Node.js 22 (via nvm, 设为默认)
- pnpm 10.28.2
- Go 1.26.3
- Docker (via Colima)
- Multica CLI v0.3.6

本地 `.env` 已配置 `DATABASE_URL` 指向远程服务器 PG。

---

## 架构图

```
团队成员浏览器
       │
       ▼
┌─────────────────────────────────────────────┐
│  服务器 49.232.233.9                         │
│                                             │
│  Nginx (:80)                                │
│    ├── / → Next.js (:3000)                  │
│    ├── /_next/static/ → 直接 serve 文件     │
│    ├── /auth/*, /api/ → Go 后端 (:8080)     │
│    └── /ws → WebSocket 后端 (:8080)         │
│                                             │
│  PostgreSQL (:5432, Docker host网络)         │
│                                             │
│  Multica Daemon → OpenCode → doubao-seed    │
└─────────────────────────────────────────────┘

本地开发:
┌─────────────────────────────────────────────┐
│  开发者电脑                                  │
│                                             │
│  方式A: 全栈本地开发                         │
│    make dev (后端+前端+本地PG)              │
│                                             │
│  方式B: 前端开发，后端用服务器               │
│    .env 设 REMOTE_API_URL=http://49.232.233.9:8080 │
│    pnpm dev:web                             │
│                                             │
│  方式C: 后端开发，连远程PG                   │
│    .env 设 DATABASE_URL 指向服务器           │
│    make server                              │
└─────────────────────────────────────────────┘
```

---

## 日常操作

### 启动/重启服务器上的服务

```bash
ssh ubuntu@49.232.233.9

# 后端
cd /opt/multica && nohup env $(cat .env | xargs) multica-server > server.log 2>&1 &

# 前端
cd /opt/multica-web/.next/standalone && nohup env PORT=3000 HOSTNAME=0.0.0.0 node apps/web/server.js > /opt/multica-web/web.log 2>&1 &

# Daemon
export PATH=$PATH:/usr/local/go/bin:/home/ubuntu/go/bin
multica daemon start

# 查看状态
multica daemon status
curl http://localhost:8080/health
curl http://localhost:3000

# 查看日志
tail -f /opt/multica/server.log          # 后端
tail -f /opt/multica-web/web.log         # 前端
tail -f ~/.multica/daemon.log            # Daemon
```

### 本地开发

```bash
cd ~/test/multica

# 方式A: 全栈本地 (需先停本地 PostgreSQL 16: brew services stop postgresql@16)
make dev

# 方式B: 只改前端，API 走服务器
# 确保 .env 里 REMOTE_API_URL=http://49.232.233.9:8080
pnpm dev:web

# 方式C: 只改后端
# 确保 .env 里 DATABASE_URL 指向 49.232.233.9
make server
```

### 部署前端更新到服务器

```bash
# 本地构建
STANDALONE=true pnpm --filter @multica/web build

# 打包
cd apps/web && tar -czf /tmp/multica-web.tar.gz .next/standalone .next/static public

# 上传
scp /tmp/multica-web.tar.gz ubuntu@49.232.233.9:/tmp/

# 在服务器上更新
ssh ubuntu@49.232.233.9
pkill -f 'node apps/web/server.js'
cd /opt/multica-web && rm -rf .next
tar -xzf /tmp/multica-web.tar.gz
cp -r .next/static .next/standalone/apps/web/.next/static
cd .next/standalone && PORT=3000 HOSTNAME=0.0.0.0 nohup node apps/web/server.js > /opt/multica-web/web.log 2>&1 &
```

### 部署后端更新到服务器

```bash
# 本地交叉编译
cd server && GOOS=linux GOARCH=amd64 go build -o /tmp/multica-server ./cmd/server

# 上传并替换
scp /tmp/multica-server ubuntu@49.232.233.9:/tmp/
ssh ubuntu@49.232.233.9
pkill multica-server
mv /tmp/multica-server /usr/local/bin/multica-server && chmod +x /usr/local/bin/multica-server
cd /opt/multica && nohup env $(cat .env | xargs) multica-server > server.log 2>&1 &
```

### 数据库 Migration

```bash
# 本地编译 migrate 工具
cd server && GOOS=linux GOARCH=amd64 go build -o /tmp/multica-migrate ./cmd/migrate
scp /tmp/multica-migrate ubuntu@49.232.233.9:/usr/local/bin/

# 在服务器上执行
ssh ubuntu@49.232.233.9
DATABASE_URL="postgres://multica:multica@127.0.0.1:5432/multica?sslmode=disable" multica-migrate up

# 或者直接从本地执行 (需要网络通)
DATABASE_URL="postgres://multica:multica@49.232.233.9:5432/multica?sslmode=disable" go run ./cmd/migrate up
```

---

## 腾讯云安全组已开放端口

| 端口 | 用途 |
|------|------|
| 22 | SSH |
| 80 | Nginx (前端+API) |
| 5432 | PostgreSQL (供本地开发连接) |
| 8080 | Go 后端直连 (可选) |

---

## 已知注意事项

1. **服务器内存**: 3.3GB，当前可用约 1.9GB。如果 agent 任务多或内存不足，考虑升配或加 swap。

2. **服务器网络**: 国内腾讯云，GitHub 下载慢。用 `ghfast.top` 做 GitHub 加速，Go 模块用 `GOPROXY=https://goproxy.cn,direct`。

3. **Docker 镜像加速**: 已配置 `/etc/docker/daemon.json` 使用国内镜像。

4. **PostgreSQL 安全**: 端口 5432 对外开放，密码是简单的 `multica`。上线前建议改强密码或限制 IP 白名单。

5. **服务自动重启**: 当前所有服务是 nohup 后台运行，重启服务器后需手动启动。后续建议写 systemd service 做自动拉起。

6. **本地 PostgreSQL 16**: macOS 上 Homebrew 装的 PostgreSQL 16 已停止 (brew services stop postgresql@16)，避免和 Docker PG 端口冲突。如果做全栈本地开发 (`make dev`)，用的是 Docker 里的 PG。

7. **Multica Cloud vs Self-host**: 本地 macOS 上的 `~/.multica/config.json` 仍连接 multica.ai (Cloud)。服务器上的 daemon 连的是自建后端 (localhost:8080)。两者独立互不影响。

---

## 后续待完成

- [ ] 将服务器上的服务注册为 systemd 服务 (自动重启)
- [ ] 配置域名 + HTTPS (Let's Encrypt)
- [ ] 将 `MULTICA_DEV_VERIFICATION_CODE` 替换为真实邮件发送 (配置 RESEND_API_KEY)
- [ ] PostgreSQL 设置强密码，安全组限制 IP
- [ ] 增加 swap 文件 (应对内存不足)
- [ ] OpenCode + doubao 在 Multica 中端到端验证 (分配 issue 给 agent 完成)
- [ ] 二次开发: 根据业务需求修改前后端代码
