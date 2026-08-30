---
title: Docker 与 OCI 存储库
order: 3
category: 指南
description: 创建镜像并使用 Docker、Podman、containerd 或 nerdctl 连接 RenoP
---

# Docker 与 OCI 存储库指南

先创建格式为 `docker` 的存储库，再在推送前创建每个目标镜像。示例使用存储库 `containers` 与镜像
`team/service`，完整 Registry 名称为 `containers/team/service`。

## 登录与传输

```bash
docker login localhost:3000
# Username: admin
# Password: <your_password_or_API_token>
```

建议使用专用 API Token：拉取使用 `repository:read`，推送使用 `repository:publish`，远程删除使用
`repository:delete`，通过管理 API 创建镜像使用 `package:create`，管理协作者使用 `team:manage`。短期 Docker
Token 只包含权限/目标限制与镜像当前 L0-L4 策略共同允许的动作。

生产环境应使用 HTTPS。仅本地 HTTP 测试时配置：

```json
{
  "insecure-registries": ["localhost:3000"]
}
```

修改 `daemon.json` 后需重启 Docker daemon。Podman 与 containerd 具有对应的 Registry 信任设置。

## 创建、标记与推送

打开 `containers`，创建 `team/service` 并选择公开或私有。私有镜像不会隐式授予 L0，应从团队面板添加只读
用户或协作者。名称路径段必须使用小写。

本地或适用的已启用上游中存在同名镜像时，创建会被拒绝；上游检查无法确定时也不会占用名称。镜像发现得到的
上游镜像保持只读。

```bash
# Tag local image
docker tag service:latest localhost:3000/containers/team/service:1.0.0

# Push image to RenoP
docker push localhost:3000/containers/team/service:1.0.0
```

镜像创建前，RenoP 不会授予推送动作，也不会开始 Blob 上传或接受 Manifest。管理请求失败后可直接重试，无需
重新登录或重开浏览器窗口。

启用任一审核策略后，创建镜像会返回 `202 Accepted`，且批准前不会占用名称。`new_packages` 模式下，后续推送
正常执行；`every_version` 模式还会将每次 Manifest 推送送审，并返回审核 ID。批准前，Manifest 与标签不会出现
在拉取、标签列表和目录响应中。批准时重新检查发布者与引用 Blob，再以原子事务发布标签。拒绝只丢弃虚拟
Manifest，不影响共享 Blob 或已有标签。镜像源导入不进入审核流程。

## 拉取与运行

```bash
# Pull image
docker pull localhost:3000/containers/team/service:1.0.0

# Run container
docker run -d -p 8080:8080 localhost:3000/containers/team/service:1.0.0
```

公开镜像可匿名读取。私有镜像要求 L0-L4 成员或管理员。Blob 权限按镜像隔离，知道其他镜像的 Digest 不会获得
访问权限。

## OCI 行为

- **多架构**：Manifest List 与 OCI Index 可引用 amd64、arm64 等平台；
- **分块上传**：支持可恢复 POST/PATCH/PUT 与有界临时存储；
- **跨仓库挂载**：要求源镜像读取权限与已预创建目标镜像写入权限；
- **删除**：同时要求 Token 能力与镜像/存储库授权；
- **镜像**：流式获取并记录上游来源，禁止向镜像制品推送。
