package encrypt

import (
	"context"
	"errors"

	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
)

// MasterEncryptKeyVault is a vault for the master encrypt key.
type MasterEncryptKeyVault interface {
	GetMasterKey(ctx context.Context) ([]byte, error)
}

func NewMasterEncryptKeyVault(setting setting.Provider) MasterEncryptKeyVault {
	return &settingMasterEncryptKeyVault{setting: setting}
}

// settingMasterEncryptKeyVault is a vault for the master encrypt key that gets the key from the setting KV.
type settingMasterEncryptKeyVault struct {
	setting setting.Provider
}

func (v *settingMasterEncryptKeyVault) GetMasterKey(ctx context.Context) ([]byte, error) {
	key := v.setting.MasterEncryptKey(ctx)
	if key == nil {
		return nil, errors.New("master encrypt key is not set")
	}
	return key, nil
}
