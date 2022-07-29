package sls

import (
	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/mitchellh/mapstructure"
)

func DecodeNetworkExtraProperties(extraPropertiesRaw interface{}, extraProperties *sls_common.NetworkExtraProperties) error {
	// Map this network to a usable structure.
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.StringToIPHookFunc(),
		Result:     extraProperties,
	})
	if err != nil {
		return err
	}

	return decoder.Decode(extraPropertiesRaw)
}
