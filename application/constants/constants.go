package constants

// These values will be injected at build time, DO NOT EDIT.

// BackendVersion 当前后端版本号
// NOTE: 该版本号同时是数据库 schema 版本标记（见 inventory.InitializeDBClient）。
// 每次发版必须与 git 标签一致：application/statics 会比对内嵌前端 version.json 中的
// 版本号，不一致则拒绝提供界面。
//
// 版本历史：
//   - 4.15.0 引入 group_storage_policies 多对多关联与回填补丁；
//   - 4.16.0 加入目录首选存储策略与存储策略间迁移（无新增补丁）；
//   - 4.17.0 修复迁移到 S3 策略时的崩溃与文件级策略不同步，并完善任务展示。
//
// 补丁执行按 Patch.EndVersion 与数据库中已记录的版本比较，因此升级版本号
// 不会重跑旧补丁。
var BackendVersion = "4.17.0"

// IsPro 是否为Pro版本
var IsPro = "false"

var IsProBool = IsPro == "true"

// LastCommit 最后commit id
var LastCommit = "000000"

const (
	APIPrefix      = "/api/v4"
	APIPrefixSlave = "/api/v4/slave"
	CrHeaderPrefix = "X-Cr-"
)

const CloudreveScheme = "cloudreve"

type (
	FileSystemType string
)

const (
	FileSystemMy           = FileSystemType("my")
	FileSystemShare        = FileSystemType("share")
	FileSystemTrash        = FileSystemType("trash")
	FileSystemSharedWithMe = FileSystemType("shared_with_me")
	FileSystemUnknown      = FileSystemType("unknown")
)
