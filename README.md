# IoT Platform

> ⚠️ An experimental project built with vibe coding.

A lightweight IoT platform built with Go, featuring device management, MQTT messaging, real-time WebSocket updates, and web dashboard.

## 目录

- [1. 环境要求](#1-环境要求)
- [2. 项目结构](#2-项目结构)
- [3. 编译步骤](#3-编译步骤)
- [4. 配置步骤](#4-配置步骤)
- [5. 启动步骤](#5-启动步骤)
- [6. 访问服务](#6-访问服务)
- [7. MQTT 连接测试](#7-mqtt-连接测试)
- [8. API 使用示例](#8-api-使用示例)
- [9. 防火墙配置](#9-防火墙配置)
- [10. Systemd 服务配置](#10-systemd-服务配置)
- [11. Docker 部署](#11-docker-部署)
- [12. 常见问题排查](#12-常见问题排查)
- [13. 安全建议](#13-安全建议)

---

## 1. 环境要求

### 硬件/软件要求

- **操作系统**: Linux (Ubuntu 20.04+), macOS, Windows (WSL)
- **Go 版本**: Go 1.21 或更高
- **网络端口**:
  - HTTP/API: 8080
  - MQTT Broker: 1883
  - WebSocket: 8080 (与 HTTP 共用)

### 安装 Go

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install golang-go

# macOS (使用 Homebrew)
brew install go

# 验证安装
go version
```

---

## 2. 项目结构

```
iot-platform/
├── cmd/
│   └── main.go              # 程序入口
├── config.yaml              # 配置文件
├── internal/
│   ├── api/
│   │   └── server.go        # REST API 服务
│   ├── config/
│   │   └── config.go        # 配置加载
│   ├── device/
│   │   └── manager.go       # 设备管理
│   ├── mqtt/
│   │   └── server.go        # MQTT Broker
│   ├── storage/
│   │   └── store.go         # 数据存储
│   └── websocket/
│       └── hub.go           # WebSocket Hub
├── pkg/
│   └── models/
│       └── models.go        # 数据模型
├── web/
│   ├── dashboard.html       # Web 仪表盘
│   └── api.html             # API 文档
├── go.mod
├── go.sum
└── iot.db                   # SQLite 数据库 (运行时自动创建)
```

---

## 3. 编译步骤

### 3.1 克隆代码

```bash
git clone <repository-url>
cd iot-platform
```

### 3.2 安装依赖

```bash
go mod download
```

### 3.3 编译

```bash
# Linux/macOS
go build -o iot-platform ./cmd/main.go

# Windows (使用 WSL 或 PowerShell)
go build -o iot-platform.exe ./cmd/main.go
```

编译成功后会在当前目录生成 `iot-platform` 可执行文件。

---

## 4. 配置步骤

### 4.1 配置文件 config.yaml

```yaml
server:
  http_port: 8080    # HTTP/WebSocket 端口
  ws_port: 8081      # (保留,实际共用8080)

mqtt:
  port: 1883         # MQTT Broker 端口
  host: "0.0.0.0"   # 监听地址

database:
  type: "sqlite"     # 数据库类型
  path: "iot.db"     # 数据库文件

device:
  heartbeat_interval: 30    # 心跳间隔(秒)
  offline_threshold: 90    # 离线阈值(秒)
```

### 4.2 配置项说明

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `server.http_port` | 8080 | HTTP API 和 WebSocket 端口 |
| `mqtt.port` | 1883 | MQTT Broker 端口 |
| `mqtt.host` | 0.0.0.0 | MQTT 监听地址，0.0.0.0 表示接受所有连接 |
| `database.path` | iot.db | SQLite 数据库文件路径 |
| `device.heartbeat_interval` | 30 | 设备心跳间隔(秒) |
| `device.offline_threshold` | 90 | 设备离线判定时间(秒)，超过此时间未收到心跳则判定为离线 |

---

## 5. 启动步骤

### 5.1 前台运行

```bash
./iot-platform
```

### 5.2 后台运行

```bash
nohup ./iot-platform > iot.log 2>&1 &
```

### 5.3 验证服务状态

```bash
curl http://localhost:8080/health
# 返回: {"status":"ok"}
```

### 5.4 查看运行日志

```bash
# 实时查看日志
tail -f iot.log

# 查看所有日志
cat iot.log
```

### 5.5 停止服务

```bash
# 查找进程
ps aux | grep iot-platform

# 终止进程
pkill -f iot-platform
```

---

## 6. 访问服务

### 6.1 Web Dashboard

- **地址**: http://localhost:8080/web/dashboard.html
- **功能**: 设备管理、设备类型管理、设备详情查看、禁用/启用设备

### 6.2 API 文档

- **地址**: http://localhost:8080/web/api.html
- **功能**: REST API 文档、MQTT 主题说明、WebSocket 事件说明

### 6.3 API 基础地址

- **地址**: http://localhost:8080/api/v1

---

## 7. MQTT 连接测试

### 7.1 安装 mosquitto_clients

```bash
# Ubuntu/Debian
sudo apt install mosquitto-clients

# macOS
brew install mosquitto

# CentOS/RHEL
sudo yum install mosquitto
```

### 7.2 测试流程

#### 步骤 1: 注册设备

```bash
curl -X POST http://localhost:8080/api/v1/devices \
  -H "Content-Type: application/json" \
  -d '{"Name":"test-device","Type":"sensor"}'
```

返回示例:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "test-device",
  "type_id": 1,
  "type_name": "sensor",
  "status": "offline",
  "secret": "fd144fe0-cf17-46eb-81ce-5fedc21f57e5",
  "disabled": false
}
```

#### 步骤 2: 连接 MQTT 并发送数据

```bash
# 发送遥测数据
mosquitto_pub \
  -h localhost -p 1883 \
  -u <device_id> \
  -P <secret> \
  -t devices/<device_id>/telemetry \
  -m '{"temperature":25.5,"humidity":60}'

# 发送心跳
mosquitto_pub \
  -h localhost -p 1883 \
  -u <device_id> \
  -P <secret> \
  -t devices/<device_id>/heartbeat \
  -m '{"status":"online"}'
```

### 7.3 认证说明

| 字段 | 说明 |
|------|------|
| 用户名 (Username) | 设备 ID (UUID)，注册时生成 |
| 密码 (Password) | 设备密钥 (Secret)，注册时生成 |

### 7.4 MQTT 主题列表

| 主题 | 方向 | 描述 |
|------|------|------|
| `devices/{id}/telemetry` | 设备 → 平台 | 遥测数据上报 |
| `devices/{id}/heartbeat` | 设备 → 平台 | 设备心跳（可包含属性数据） |
| `devices/{id}/status` | 设备 → 平台 | 状态更新 |
| `devices/{id}/command` | 平台 → 设备 | 下发命令 |
| `devices/{id}/command/resp` | 设备 → 平台 | 命令响应 |

---

## 8. API 使用示例

### 8.1 设备管理

```bash
# 列出所有设备
curl http://localhost:8080/api/v1/devices

# 获取设备详情
curl http://localhost:8080/api/v1/devices/<device_id>

# 更新设备信息
curl -X PUT http://localhost:8080/api/v1/devices/<device_id> \
  -H "Content-Type: application/json" \
  -d '{"Name":"new-name","Type":"actuator"}'

# 禁用设备 (断开连接)
curl -X PUT http://localhost:8080/api/v1/devices/<device_id>/disable

# 启用设备
curl -X PUT http://localhost:8080/api/v1/devices/<device_id>/enable

# 删除设备 (仅离线设备)
curl -X DELETE http://localhost:8080/api/v1/devices/<device_id>
```

### 8.2 设备类型管理

```bash
# 列出所有类型
curl http://localhost:8080/api/v1/device-types

# 创建新类型
curl -X POST http://localhost:8080/api/v1/device-types \
  -H "Content-Type: application/json" \
  -d '{"Name":"custom-type"}'

# 更新类型
curl -X PUT http://localhost:8080/api/v1/device-types/<type_id> \
  -H "Content-Type: application/json" \
  -d '{"Name":"new-name"}'

# 删除类型 (有设备时无法删除)
curl -X DELETE http://localhost:8080/api/v1/device-types/<type_id>
```

### 8.3 遥测数据

```bash
# 获取设备遥测数据
curl "http://localhost:8080/api/v1/devices/<device_id>/telemetry?limit=100"

# 获取命令历史
curl "http://localhost:8080/api/v1/devices/<device_id>/commands?limit=50"
```

### 8.4 发送命令

```bash
curl -X POST http://localhost:8080/api/v1/devices/<device_id>/command \
  -H "Content-Type: application/json" \
  -d '{"Command":"restart","Params":{"delay":5}}'
```

### 8.5 统计信息

```bash
# 获取平台统计
curl http://localhost:8080/api/v1/stats
```

---

## 9. 防火墙配置

### 9.1 Ubuntu/Debian (UFW)

```bash
# 开放端口
sudo ufw allow 8080/tcp  # HTTP/API
sudo ufw allow 1883/tcp  # MQTT

# 重启防火墙
sudo ufw reload

# 查看规则
sudo ufw status
```

### 9.2 CentOS/RHEL (firewalld)

```bash
# 永久开放端口
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=1883/tcp

# 重载防火墙
sudo firewall-cmd --reload

# 查看规则
sudo firewall-cmd --list-ports
```

### 9.3 macOS

```bash
# macOS 通常不需要额外配置防火墙
# 如有需要，通过系统偏好设置 → 安全性与隐私 → 防火墙
```

### 9.4 Windows (WSL)

```powershell
# 在 Windows 防火墙中开放端口
netsh advfirewall firewall add rule name="IoT Platform HTTP" dir=in action=allow protocol=tcp localport=8080
netsh advfirewall firewall add rule name="IoT Platform MQTT" dir=in action=allow protocol=tcp localport=1883
```

---

## 10. Systemd 服务配置

### 10.1 创建服务文件

```bash
sudo nano /etc/systemd/system/iot-platform.service
```

### 10.2 服务配置内容

```ini
[Unit]
Description=IoT Platform
After=network.target

[Service]
Type=simple
User=<username>
WorkingDirectory=/path/to/iot-platform
ExecStart=/path/to/iot-platform/iot-platform
Restart=on-failure
RestartSec=5
StandardOutput=append:/var/log/iot-platform.log
StandardError=append:/var/log/iot-platform.log

[Install]
WantedBy=multi-user.target
```

### 10.3 启用服务

```bash
# 重新加载 systemd
sudo systemctl daemon-reload

# 启用服务 (开机自启)
sudo systemctl enable iot-platform

# 启动服务
sudo systemctl start iot-platform

# 查看服务状态
sudo systemctl status iot-platform

# 查看日志
sudo journalctl -u iot-platform -f
```

---

## 11. Docker 部署

### 11.1 创建 Dockerfile

```dockerfile
# 构建阶段
FROM golang:1.21-alpine AS builder

WORKDIR /app

# 复制 go mod 文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码并构建
COPY . .
RUN go build -o iot-platform ./cmd/main.go

# 运行阶段
FROM alpine:latest

RUN apk add --no-cache ca-certificates bash

WORKDIR /app

# 从构建阶段复制可执行文件
COPY --from=builder /app/iot-platform .
COPY --from=builder /app/config.yaml .

# 暴露端口
EXPOSE 8080 1883

# 运行
CMD ["./iot-platform"]
```

### 11.2 构建 Docker 镜像

```bash
docker build -t iot-platform:latest .
```

### 11.3 运行容器

```bash
# 前台运行
docker run -p 8080:8080 -p 1883:1883 iot-platform:latest

# 后台运行
docker run -d -p 8080:8080 -p 1883:1883 --name iot-platform iot-platform:latest

# 查看运行日志
docker logs -f iot-platform
```

### 11.4 Docker Compose (推荐)

创建 `docker-compose.yml`:

```yaml
version: '3.8'

services:
  iot-platform:
    build: .
    ports:
      - "8080:8080"
      - "1883:1883"
    volumes:
      - ./config.yaml:/app/config.yaml
      - ./iot.db:/app/iot.db
    restart: unless-stopped
```

运行:

```bash
docker-compose up -d
docker-compose logs -f
```

---

## 12. 常见问题排查

### 12.1 服务无法启动

**症状**: 执行 `./iot-platform` 后立即退出

**排查步骤**:

```bash
# 1. 检查端口占用
lsof -i :8080
lsof -i :1883

# 2. 查看详细错误
./iot-platform 2>&1

# 3. 检查配置文件格式
cat config.yaml

# 4. 检查数据库文件权限
ls -la iot.db
```

**常见原因**:
- 端口被其他程序占用
- 配置文件格式错误 (YAML 语法)
- 数据库文件权限不足

---

### 12.2 MQTT 连接失败

**症状**: 设备无法连接到 MQTT Broker

**排查步骤**:

```bash
# 1. 检查 MQTT 服务是否启动
telnet localhost 1883

# 2. 检查防火墙
sudo ufw status  # Ubuntu
sudo firewall-cmd --list-ports  # CentOS

# 3. 检查设备认证信息
#    - 用户名应为设备 ID (UUID)
#    - 密码应为设备密钥 (Secret)

# 4. 检查设备是否被禁用
curl http://localhost:8080/api/v1/devices/<device_id>
# 查看返回中 "disabled" 字段是否为 true
```

**常见原因**:
- 防火墙未开放 1883 端口
- 设备 ID 或 Secret 错误
- 设备已被禁用

---

### 12.3 设备属性显示为空

**症状**: Dashboard 中设备详情页的 Properties 项为空

**排查步骤**:

```bash
# 1. 检查 MQTT 消息是否正确发送
#    遥测数据应发送到: devices/<device_id>/telemetry

# 2. 查看服务端日志
tail -f iot.log | grep telemetry

# 3. 检查遥测数据 API
curl "http://localhost:8080/api/v1/devices/<device_id>/telemetry?limit=5"
```

**常见原因**:
- MQTT 消息主题格式不正确
- MQTT QoS 设置问题
- 设备发送的 JSON 格式错误

---

### 12.4 WebSocket 连接失败

**症状**: Dashboard 页面无法实时更新设备状态

**排查步骤**:

```bash
# 1. 检查 WebSocket 服务
curl -i http://localhost:8080/ws

# 2. 检查浏览器控制台错误
#    - 打开浏览器开发者工具 (F12)
#    - 查看 Console 和 Network 标签页

# 3. 检查端口 8080 是否正常
curl http://localhost:8080/health
```

**常见原因**:
- 浏览器缓存旧版本页面 (尝试 Ctrl+F5 强制刷新)
- WebSocket 被企业防火墙阻止
- 端口 8080 未正确开放

---

### 12.5 数据库问题

**症状**: 服务启动失败或数据丢失

**排查步骤**:

```bash
# 1. 检查数据库文件是否存在
ls -la iot.db

# 2. 备份并重建数据库
cp iot.db iot.db.backup
rm iot.db
./iot-platform  # 重新初始化

# 3. 检查数据库文件权限
chmod 644 iot.db
```

**常见原因**:
- 数据库文件损坏
- 磁盘空间不足
- 文件权限问题

---

### 12.6 设备删除失败

**症状**: 无法删除设备

**排查步骤**:

```bash
# 1. 确认设备状态
curl http://localhost:8080/api/v1/devices/<device_id>
# 查看 "status" 字段

# 2. 设备必须先离线才能删除
#    - 如果在线，先调用禁用接口
curl -X PUT http://localhost:8080/api/v1/devices/<device_id>/disable
```

**常见原因**:
- 设备处于在线状态
- 设备连接未断开

---

## 13. 安全建议

### 生产环境部署

1. **启用 TLS/SSL 加密**
   - HTTP API 使用 HTTPS
   - MQTT 使用 MQTT over TLS (port 8883)

2. **配置用户认证系统**
   - 添加 API Key 认证
   - 添加 Dashboard 登录功能

3. **限制 MQTT 访问**
   - 配置 IP 白名单
   - 使用防火墙规则限制来源 IP

4. **API 请求限流**
   - 防止恶意请求
   - 配置 QOS 限制

5. **日志持久化**
   - 配置日志轮转
   - 定期归档日志文件

### 数据安全

1. **定期备份数据库**
   ```bash
   cp iot.db iot.db.$(date +%Y%m%d)
   ```

2. **数据库文件权限**
   ```bash
   chmod 600 iot.db
   ```

3. **敏感信息保护**
   - 不要将 `iot.db` 或 `config.yaml` 提交到代码仓库
   - 使用环境变量存储敏感配置

---

## 14. 技术支持

- **API 文档**: http://localhost:8080/web/api.html
- **状态检查**: http://localhost:8080/health
- **Web Dashboard**: http://localhost:8080/web/dashboard.html

---

## 版本信息

- **当前版本**: 1.0.0
- **Go 版本**: 1.21+
- **最后更新**: 2026-04-06
