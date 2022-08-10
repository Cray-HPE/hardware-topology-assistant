package sls

import (
	"fmt"

	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/Cray-HPE/hms-xname/xnametypes"
	"github.com/mitchellh/mapstructure"
	"github.hpe.com/sjostrand/topology-tool/pkg/configs"
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

func FindManagementNCNs(allHardware map[string]sls_common.GenericHardware) ([]sls_common.GenericHardware, error) {
	var managementNCNs []sls_common.GenericHardware

	for _, hardware := range allHardware {
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

// FilterHardware will apply the given filter to a map of generic hardware
func FilterHardware(allHardware map[string]sls_common.GenericHardware, filter func(sls_common.GenericHardware) (bool, error)) (map[string]sls_common.GenericHardware, error) {
	result := map[string]sls_common.GenericHardware{}

	for xname, hardware := range allHardware {
		ok, err := filter(hardware)
		if err != nil {
			return nil, err
		}

		if ok {
			result[xname] = hardware
		}
	}

	return result, nil
}

func FilterOutManagementNCNs(allHardware map[string]sls_common.GenericHardware) (map[string]sls_common.GenericHardware, error) {
	// Find all of the Management NCNs
	managementNCNs, err := FindManagementNCNs(allHardware)
	if err != nil {
		return nil, err
	}

	// Build up some lookup Maps
	isManagementNCN := map[string]bool{}
	isManagementNCNBMC := map[string]bool{}
	for _, hardware := range managementNCNs {
		isManagementNCN[hardware.Xname] = true
		isManagementNCN[xnametypes.GetHMSCompParent(hardware.Xname)] = true
	}

	return FilterHardware(allHardware, func(hardware sls_common.GenericHardware) (bool, error) {
		// Check to see if this is a Management NCN
		if isManagementNCN[hardware.Xname] {
			return false, nil
		}

		// Check to see if this is a MgmtSwitchConnector for a Management NCN BMC
		if hardware.TypeString == xnametypes.MgmtSwitchConnector {
			var extraProperties sls_common.ComptypeMgmtSwitchConnector
			if err := mapstructure.Decode(hardware.ExtraPropertiesRaw, &extraProperties); err != nil {
				return false, err
			}

			for _, nodeNic := range extraProperties.NodeNics {
				if isManagementNCNBMC[nodeNic] {
					return false, nil
				}
			}
		}

		// This is not a Management NCN!
		return true, nil
	})
}

func BuildApplicationNodeMetadata(allHardware map[string]sls_common.GenericHardware) (configs.ApplicationNodeMetadataMap, error) {
	metadata := configs.ApplicationNodeMetadataMap{}

	// Find all application nodes
	for _, hardware := range allHardware {

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

		if nodeEP.Role != "Application" {
			continue
		}

		// Found an application node!
		metadata[hardware.Xname] = configs.ApplicationNodeMetadata{
			SubRole: nodeEP.SubRole,
			Aliases: nodeEP.Aliases,
		}
	}

	return metadata, nil
}

func SwitchAliases(allHardware map[string]sls_common.GenericHardware) (map[string][]string, error) {
	result := map[string][]string{}

	// Find all switches
	for _, hardware := range allHardware {
		extraPropertiesRaw, err := DecodeHardwareExtraProperties(hardware)
		if err != nil {
			return nil, err
		}

		switch extraProperties := extraPropertiesRaw.(type) {
		case sls_common.ComptypeMgmtSwitch:
			result[hardware.Xname] = extraProperties.Aliases
		case sls_common.ComptypeMgmtHLSwitch:
			result[hardware.Xname] = extraProperties.Aliases
		}
	}

	return result, nil
}
