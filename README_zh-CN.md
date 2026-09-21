[English Version](https://github.com/cloudreve/cloudreve/blob/master/README.md)

<h1 align="center">
  <br>
  <a href="https://cloudreve.org/" alt="logo" ><img src="https://raw.githubusercontent.com/cloudreve/frontend/master/public/static/img/logo192.png" width="150"/></a>
  <br>
  Cloudreve
  <br>
</h1>

<h4 align="center">支持多家云存储驱动的公有云文件系统.</h4>

<p align="center">
  <a href="https://dev.azure.com/abslantliu/cloudreve/_build?definitionId=6">
    <img src="https://img.shields.io/github/check-runs/cloudreve/cloudreve/master"
         alt="Azure pipelines">
  </a>
  <a href="https://github.com/cloudreve/cloudreve/releases">
    <img src="https://img.shields.io/github/v/release/cloudreve/cloudreve?include_prereleases" />
  </a>
  <a href="https://github.com/cloudreve/cloudreve/releases">
     <img src="https://badgen.net/static/release%20size/34%20MB/blue"/>
  </a>
  <a href="https://hub.docker.com/r/cloudreve/cloudreve">
  <img alt="Docker Pulls" src="https://img.shields.io/docker/pulls/cloudreve/cloudreve" />
  </a>
</p>
<p align="center">
  <a href="https://cloudreve.org">主页</a> •
  <a href="https://demo.cloudreve.org">演示</a> •
  <a href="https://github.com/cloudreve/cloudreve/discussions">讨论</a> •
  <a href="https://docs.cloudreve.org">文档</a> •
  <a href="https://github.com/cloudreve/cloudreve/releases">下载</a> •
  <a href="https://t.me/cloudreve_official">Telegram</a> •
  <a href="https://discord.com/invite/WTpMFpZT76">Discord</a>
</p>

![Screenshot](https://raw.githubusercontent.com/cloudreve/docs/master/images/homepage.png)

## :sparkles: 特性

- :cloud: 支持本机、从机、七牛 Kodo、阿里云 OSS、腾讯云 COS、华为云 OBS、金山云 KS3、又拍云、OneDrive (包括世纪互联版) 、S3 兼容协议 作为存储端
- :outbox_tray: 上传/下载 支持客户端直传，支持下载限速
- 💾 可对接 Aria2/qBittorrent 离线下载，可使用多个从机节点分担下载任务
- 📚 在线 压缩/解压缩/压缩包预览、多文件打包下载
- 💻 覆盖全部存储策略的 WebDAV 协议支持
- :zap: 拖拽上传、目录上传、并行分片上传
- :card_file_box: 提取媒体元数据，通过元数据或标签搜索文件
- :family_woman_girl_boy: 多用户、用户组、多存储策略
- :link: 创建文件、目录的分享链接，可设定自动过期
- :eye_speech_bubble: 视频、图像、音频、 ePub 在线预览，文本、Office 文档在线编辑
- :art: 自定义配色、黑暗模式、PWA 应用、全站单页应用、国际化支持
- :rocket: All-in-One 打包，开箱即用
- 🌈 ... ...

## :hammer_and_wrench: 部署

你可以参考 [快速开始](https://docs.cloudreve.org/overview/quickstart) 启动一个本地实例进行体验、测试。

当你准备好将 Cloudreve 部署到生产环境时，可以参考 [部署](https://docs.cloudreve.org/overview/deploy/) 进行完整部署。

## :gear: 构建

你可以参考 [构建](https://docs.cloudreve.org/overview/build/) 从源代码构建 Cloudreve。

## :bookmark: 本仓库说明（fork）

本仓库是 `cloudreve/Cloudreve` 的私有 fork，用于内部部署。**上游文档描述的行为与本仓库的实际行为并不完全一致**，改动与流程以下文为准。

### 与上游的差异

1. **一个用户组可绑定多个存储策略**（多对多关联 `group_storage_policies`，旧的单选列保留但已废弃）；
2. **目录级「首选存储策略」**：上传时沿父目录链查找，落到所选策略；
3. **文件/目录可在存储策略之间迁移**（迁移任务 + 回滚）；
4. **前端源码在 `assets/` 且已被普通提交**（上游是 git submodule，本仓库已移除 `.gitmodules`）；
5. **发布方式不同**：不再使用 goreleaser/Azure Pipelines，改为 GitHub Actions 发布容器镜像（见下）。

### 版本号规则

`application/constants/constants.go` 中的 `BackendVersion` **同时是**：

- 数据库 schema 版本标记（`inventory.InitializeDBClient`）；
- 与内嵌前端 `version.json` 比对的版本号。

因此**改版本号必须同时改三处，否则启动会报 `Static resource version mismatch`**：

| 位置 | 说明 |
|---|---|
| `application/constants/constants.go` → `BackendVersion` | 二进制内嵌版本 |
| `assets/build-frontend.ps1` → `$BackendVersion` 默认参数 | 本地打包前端时盖的版本 |
| git 标签 | CI 会用标签覆盖 `version.json`，所以标签必须与上面一致 |

版本号**不带 `v` 前缀**，形如 `4.16.0`。本 fork 占住**次版本号**（如 4.16.0）以区别于上游的补丁版本（4.15.1、4.15.2…），避免将来合并上游时命名混淆。

> 数据库补丁按 `Patch.EndVersion` 与数据库中已记录的版本比较（见 `inventory/migration.go`），因此单纯升版本号**不会**重跑旧补丁；只有需要新补丁时才往 `patches` 里加条目。

### 发版流程

```bash
# 1. 改版本号（上面三处里的两处，第三处是下面的标签）
#    application/constants/constants.go
#    assets/build-frontend.ps1

# 2. 本地验证（见「本地构建与测试」），然后提交
git add -A && git commit -m "release: bump BackendVersion to 4.17.0"
git push origin main

# 3. 打标签并推送 —— 这一步触发发布
git tag -a 4.17.0 -m "Cloudreve 4.17.0 fork release"
git push origin 4.17.0
```

标签推送后 `.github/workflows/release-image.yml` 会自动：

1. 安装前端依赖并 `vite build`，用标签覆盖 `build/version.json`，打包 `application/statics/assets.zip`；
2. 用 Buildx 构建 `linux/amd64` + `linux/arm64` 多架构镜像，推送到 `ghcr.io/shinyes/cloudreve`，标签为 `<版本>`、`latest`、`v4`；
3. 分别构建两个架构并用 `docker save` + `gzip` 导出，作为 release 附件上传：
   `cloudreve_<版本>_linux_amd64.tar.gz`、`cloudreve_<版本>_linux_arm64.tar.gz`；
4. 创建对应 tag 的 GitHub Release 并附上这两个压缩包。

**注意**：标签若写成 `v4.17.0`，既不匹配触发规则（`[0-9]+.[0-9]+.[0-9]+`）会静默不运行，也会因为与 `version.json` 不一致而导致启动报错。

前置条件（首次发版可能需要在 GitHub 网页上开）：私有仓库的 `GITHUB_TOKEN` 默认只读，需在 *Settings → Actions → General → Workflow permissions* 勾选 **Read and write permissions**；镜像推送成功后包默认为私有，需在 *Packages → cloudreve → Package settings* 设为 public。

### 本地构建与测试

`application/statics/assets.zip` **被 .gitignore 忽略**（由 CI 生成）。所以新 clone 后直接 `go build` 会因 `//go:embed` 找不到文件而失败，必须先构建前端：

```powershell
# 构建前端并打包 assets.zip（会同时盖版本号）
powershell -NoProfile -ExecutionPolicy Bypass -File assets\build-frontend.ps1 -BackendVersion 4.16.0
go build -o cloudreve.exe .
```

测试：

```powershell
# Go 单元测试：加密元数据契约、迁移的加密矩阵
go test ./inventory/ ./pkg/filemanager/manager/

# 端到端：三套件，各自起临时实例
powershell -NoProfile -ExecutionPolicy Bypass -File tests\relocate\run.ps1
```

前端构建**不要用 `npm run build-prod`**（它会先跑 `tsc`，而仓库存在与本次改动无关的历史类型错误，必然失败）；CI 与本地都用 `npm run build`。

### 与上游的关系（重要）

本 fork 基于上游 **4.19.1**，且**永不向上游发布任何内容**：不提 PR、不推分支、不推标签。详见 [`UPSTREAM_POLICY.md`](UPSTREAM_POLICY.md)。

- `upstream` 配置为**只读镜像**（`pushurl` 指向无效占位），因此既能共享 git 祖先、又无法推送；
- `.githooks/pre-push` 会拒绝推送到上游 URL（新 clone 需执行一次 `git config core.hooksPath .githooks` 启用）；
- `.github/workflows/upstream-guard.yml` 在每次 push/PR 时校验两件事：**没有任何 remote 能推送到上游**，且**与上游的共享祖先仍然存在**。

**改动代码前请先读 [`UPGRADING.md`](UPGRADING.md)**。其中记录了必须遵守的开发准则（**禁止重新导入上游源码**、禁止重写已发布历史、保持改动可识别），以及合并上游发布的流程——它是普通的三方合并：

```bash
git fetch upstream master --no-tags
git switch -c merge/upstream-<version> main
git merge upstream/master
```

## :rocket: 贡献

如果你有兴趣为 Cloudreve 贡献代码，请参考 [贡献](https://docs.cloudreve.org/api/contributing/) 了解如何贡献。

## :alembic: 技术栈

- [Go](https://golang.org/) + [Gin](https://github.com/gin-gonic/gin) + [ent](https://github.com/ent/ent)
- [React](https://github.com/facebook/react) + [Redux](https://github.com/reduxjs/redux) + [Material-UI](https://github.com/mui-org/material-ui)

## :scroll: 许可证

GPL V3
