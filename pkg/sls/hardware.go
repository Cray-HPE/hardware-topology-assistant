package sls

import (
	"fmt"

	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/Cray-HPE/hms-xname/xnametypes"
	"github.com/mitchellh/mapstructure"
)

func DecodeHardwareExtraProperties(hardware sls_common.GenericHardware) (result interface{}, err error) {
	// This can be filled out with types with some help of the following. Doesn't fully work, but gets you close
	// $ cat pkg/sls-common/types.go | grep '^type Comptype' | sort
	switch hardware.TypeString {
	case xnametypes.NodeBMCNic:
		result = sls_common.ComptypeBmcNic{}
	case xnametypes.CDUMgmtSwitch:
		result = sls_common.ComptypeCDUMgmtSwitch{}
	case xnametypes.CabinetPDUNic:
		result = sls_common.ComptypeCabPduNic{}
	case xnametypes.Cabinet:
		result = sls_common.ComptypeCabinet{}
	case xnametypes.ChassisBMC:
		result = sls_common.ComptypeChassisBmc{}
	case xnametypes.ComputeModule:
		result = sls_common.ComptypeCompmod{}
	case xnametypes.NodePowerConnector:
		result = sls_common.ComptypeCompmodPowerConnector{}
	case xnametypes.NodeHsnNic:
		result = sls_common.ComptypeNodeHsnNic{}
	case xnametypes.HSNConnectorPort:
		result = sls_common.ComptypeHSNConnector{}
	case xnametypes.MgmtHLSwitch:
		result = sls_common.ComptypeMgmtHLSwitch{}
	case xnametypes.MgmtSwitch:
		result = sls_common.ComptypeMgmtSwitch{}
	case xnametypes.MgmtSwitchConnector:
		result = sls_common.ComptypeMgmtSwitchConnector{}
	case xnametypes.Node:
		result = sls_common.ComptypeNode{}
	case xnametypes.NodeBMC:
		result = sls_common.ComptypeNodeBmc{}
	case xnametypes.NodeNic:
		result = sls_common.ComptypeNodeNic{}
	case xnametypes.RouterBMC:
		result = sls_common.ComptypeRtrBmc{}
	case xnametypes.RouterBMCNic:
		result = sls_common.ComptypeRtrBmcNic{}
	case xnametypes.RouterModule:
		result = sls_common.ComptypeRtrBmcNic{}
	default:
		// Not all SLS types have an associated struct. If EP is nil, then its not a problem.
		if hardware.ExtraPropertiesRaw == nil {
			return nil, nil
		}

		return nil, fmt.Errorf("hardware object (%s) has unexpected properties", hardware.Xname)
	}

	// Decode the Raw extra properties into a give structure
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.StringToIPHookFunc(),
		Result:     &result,
	})
	if err != nil {
		return nil, err
	}
	err = decoder.Decode(hardware.ExtraPropertiesRaw)

	return result, err
}

func FindManagementNCNs(slsState sls_common.SLSState) ([]sls_common.GenericHardware, error) {
	var managementNCNs []sls_common.GenericHardware

	for _, hardware := range slsState.Hardware {
		if xnametypes.GetHMSType(hardware.Xname) != xnametypes.Node {
			continue
		}

		var nodeEP sls_common.ComptypeNode
		if ep, ok := hardware.ExtraPropertiesRaw.(sls_common.ComptypeNode); ok {
			// If we are there, then the extra properties where created at runtime
			nodeEP = ep
		} else {
			// If we are there, then the extra properties came from JSON
			if err := mapstructure.Decode(hardware.ExtraPropertiesRaw, &nodeEP); err != nil {
				return nil, err
			}
		}

		if nodeEP.Role == "Management" {
			managementNCNs = append(managementNCNs, hardware)
		}
	}

	return managementNCNs, nil
}
