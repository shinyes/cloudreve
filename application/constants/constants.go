package constants

// These values will be injected at build time, DO NOT EDIT.

// BackendVersion 当前后端版本号
// NOTE: 该版本号同时是数据库 schema 版本标记（见 inventory.InitializeDBClient）。
// 4.15.0 引入 group_storage_policies 多对多关联与回填补丁，必须递增版本号，
// 否则已完成 4.14.0 迁移的数据库不会再次进入迁移流程，补丁不会执行。
var BackendVersion = "4.15.0"

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
