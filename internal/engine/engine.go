package engine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Cray-HPE/cray-site-init/pkg/csi"
	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/Cray-HPE/hms-xname/xnames"
	"github.com/Cray-HPE/hms-xname/xnametypes"
	"github.hpe.com/sjostrand/topology-tool/pkg/ccj"
	"github.hpe.com/sjostrand/topology-tool/pkg/configs"
	"github.hpe.com/sjostrand/topology-tool/pkg/ipam"
	"github.hpe.com/sjostrand/topology-tool/pkg/sls"
)

type TopologyEngine struct {
	Input EngineInput
}

type EngineInput struct {
	Paddle                ccj.Paddle
	CabinetLookup         configs.CabinetLookup
	ApplicationNodeConfig csi.SLSGeneratorApplicationNodeConfig

	CurrentSLSState sls_common.SLSState
}

type EngineResult struct {
	HardwareAdded                  []sls_common.GenericHardware
	ModifiedNetworkExtraProperties map[string]*sls_common.NetworkExtraProperties
}

func (te *TopologyEngine) DetermineChanges() (*EngineResult, error) {
	// Build up the expected SLS hardware state from the provided CCJ
	expectedSLSState, err := te.buildExpectedHardwareState()
	if err != nil {
		return nil, fmt.Errorf("failed to build expected SLS hardware state: %w", err)
	}

	//
	// Compare the current hardware state with the expected hardware state
	//

	// Identify missing hardware from either side
	hardwareRemoved, err := sls.HardwareSubtract(te.Input.CurrentSLSState, expectedSLSState)
	if err != nil {
		return nil, err
	}

	hardwareAdded, err := sls.HardwareSubtract(expectedSLSState, te.Input.CurrentSLSState)
	if err != nil {
		return nil, err
	}

	// Identify hardware present in both states
	// Does not take into account differences in Class/ExtraProperties, just by the primary key of xname
	identicalHardware, hardwareWithDifferingValues, err := sls.HardwareUnion(te.Input.CurrentSLSState, expectedSLSState)
	if err != nil {
		return nil, err
	}

	te.displayHardwareComparisonReport(hardwareRemoved, hardwareAdded, identicalHardware, hardwareWithDifferingValues)

	//
	// GUARD RAILS - If hardware is removed of has differing values then
	// DO NOT PROCEED, as those are currently out of scope use cases.
	//
	// This is put in place as the first use for this tool is to add river cabinets to the system.
	//
	if len(hardwareRemoved) != 0 {
		return nil, fmt.Errorf("refusing to continue, found hardware was removed from the system. Please reconcile the current system state with the systems CCJ/SHCD")
	}

	if len(hardwareWithDifferingValues) != 0 {
		return nil, fmt.Errorf("refusing to continue, found hardware with differing values (Class and/or ExtraProperties). Please reconcile the differences")
	}

	// TODO Disallow for mountain cabinet additions? Or for right now just ignore them all add stuff with the River class

	//
	// Check for new hardware additions that require changes to the network
	//

	// Create lookup maps for network extra properties for easier modified networks
	modifiedNetworks := map[string]bool{}
	networkExtraProperties := map[string]*sls_common.NetworkExtraProperties{}
	for networkName, slsNetwork := range te.Input.CurrentSLSState.Networks {
		var ep sls_common.NetworkExtraProperties
		if err := sls.DecodeNetworkExtraProperties(slsNetwork.ExtraPropertiesRaw, &ep); err != nil {
			return nil, fmt.Errorf("failed to decode extra properties for network (%s)", networkName)
		}

		networkExtraProperties[networkName] = &ep
	}

	// First look for any new cabinets, and allocation an subnet for them
	for i, hardware := range hardwareAdded {
		if hardware.TypeString == xnametypes.Cabinet {
			// TODO In the case of added liquid-cooled cabinets the HMN_MTN or NMN_MTN networks may not exist.
			// Such as the case of adding a liquid-cooled cabinet to a river only system.

			// Allocation of the Cabinet Subnets
			for _, networkPrefix := range []string{"HMN", "NMN"} {
				networkName, err := te.determineCabinetNetwork(networkPrefix, hardware.Class)
				if err != nil {
					return nil, err
				}

				// Retrieve the network
				networkExtraProperties, present := networkExtraProperties[networkName]
				if !present {
					return nil, fmt.Errorf("unable to allocate cabinet subnet network does not exist (%s)", networkName)
				}

				// Find an available subnet
				xnameRaw := xnames.FromString(hardware.Xname)
				xname, ok := xnameRaw.(xnames.Cabinet)
				if !ok {
					return nil, fmt.Errorf("unable to parse cabinet xname (%s)", hardware.Xname)
				}

				// TODO deal with Vlan later
				subnet, err := ipam.AllocateCabinetSubnet(*networkExtraProperties, xname, nil)
				if err != nil {
					return nil, fmt.Errorf("unable to allocate subnet for cabinet (%s) in network (%s)", hardware.Xname, networkName)
				}

				// Push in the newly created subnet into the SLS network
				networkExtraProperties.Subnets = append(networkExtraProperties.Subnets, subnet)
				modifiedNetworks[networkName] = true

				// Update the cabinet hardware object to include the updated network info
				extraProperties, ok := hardware.ExtraPropertiesRaw.(sls_common.ComptypeCabinet)
				if !ok {
					return nil, fmt.Errorf("cabinet (%s) is missing its extra properties structure", hardware.Xname)
				}

				// TODO This network information in the long term should not exist here in SLS.
				extraProperties.Networks["cn"] = map[string]sls_common.CabinetNetworks{
					"HMN": {
						CIDR:    subnet.CIDR,
						Gateway: subnet.Gateway.String(),
						VLan:    int(subnet.VlanID),
					},
				}

				if hardware.Class == sls_common.ClassRiver {
					extraProperties.Networks["ncn"] = extraProperties.Networks["cn"]
				}

				hardwareAdded[i] = hardware
			}
		}
	}

	// Allocation Switch IPs
	for i, hardware := range hardwareAdded {
		hmsType := hardware.TypeString
		if hmsType == xnametypes.CDUMgmtSwitch || hmsType == xnametypes.MgmtHLSwitch || hmsType == xnametypes.MgmtSwitch {
			// Allocation IP for Switch

			// TODO if CSM 1.0 is going to be supported at some point with this tool, the CMN network needs to become optional
			for _, networkName := range []string{"HMN", "NMN", "MTL", "CMN"} {
				// Retrieve the network
				networkExtraProperties, present := networkExtraProperties[networkName]
				if !present {
					return nil, fmt.Errorf("unable to allocate cabinet subnet network does not exist (%s)", networkName)
				}

				// Retrieve the subnet
				slsSubnet, slsSubnetIndex, err := networkExtraProperties.LookupSubnet("network_hardware")
				if !present {
					return nil, fmt.Errorf("unable to find subnet in (%s) network: %w", networkName, err)
				}

				// Parse the xname
				xname := xnames.FromString(hardware.Xname)
				if xname != nil {
					return nil, fmt.Errorf("unable to parse switch xname (%s)", hardware.Xname)
				}

				// Get the switches alias
				var aliases []string
				switch ep := hardware.ExtraPropertiesRaw.(type) {
				case sls_common.ComptypeCDUMgmtSwitch:
					aliases = ep.Aliases
				case sls_common.ComptypeMgmtHLSwitch:
					aliases = ep.Aliases
				case sls_common.ComptypeMgmtSwitch:
					aliases = ep.Aliases
				default:
					return nil, fmt.Errorf("switch (%s) has invalid or missing extra properties structure", hardware.Xname)
				}

				if len(aliases) != 0 {
					return nil, fmt.Errorf("switch (%s) has unexpected number of aliases (%d) expected 1", hardware.Xname, len(aliases))
				}

				// Allocate the IP!
				ipReservation, err := ipam.AllocateSwitchIP(slsSubnet, xname, aliases[0])
				if err != nil {
					return nil, fmt.Errorf("unable to allocate IP for switch (%s) in network (%s): %w", xname.String(), networkName, err)
				}

				// Push in the network IP Reservation into the subnet
				slsSubnet.IPReservations = append(slsSubnet.IPReservations, ipReservation)
				networkExtraProperties.Subnets[slsSubnetIndex] = slsSubnet
				modifiedNetworks[networkName] = true

				// Update the hardware object to have the HMN IP, which is required for the hms-discovery and REDS to function
				// In the future this should be phased out.
				if hmsType == xnametypes.MgmtSwitch && networkName == "HMN" {
					hardwareAdded[i] = hardware
				}
			}

		}
	}

	// Filter NetworkExtraProperties to include only the modified networks
	modifiedNetworkExtraProperties := map[string]*sls_common.NetworkExtraProperties{}
	for networkName, networkExtraProperties := range networkExtraProperties {
		if modifiedNetworks[networkName] {
			modifiedNetworkExtraProperties[networkName] = networkExtraProperties
		}
	}

	return &EngineResult{
		HardwareAdded:                  hardwareAdded,
		ModifiedNetworkExtraProperties: modifiedNetworkExtraProperties,
	}, nil
}

func (te *TopologyEngine) buildExpectedHardwareState() (sls_common.SLSState, error) {
	paddle := te.Input.Paddle
	cabinetLookup := te.Input.CabinetLookup
	applicationNodeConfig := te.Input.ApplicationNodeConfig

	// Iterate over the paddle file to build of SLS data
	allHardware := map[string]sls_common.GenericHardware{}
	for _, topologyNode := range paddle.Topology {
		fmt.Println(topologyNode.Architecture, topologyNode.CommonName)

		//
		// Build the SLS hardware representation
		//
		hardware, err := ccj.BuildSLSHardware(topologyNode, paddle, cabinetLookup, applicationNodeConfig)
		if err != nil {
			panic(err)
		}

		// Ignore empty hardware
		if hardware.Xname == "" {
			continue
		}

		// Verify cabinet exists (ignore CDUs)
		if strings.HasPrefix(hardware.Xname, "x") {
			cabinetXname, err := csi.CabinetForXname(hardware.Xname)
			if err != nil {
				panic(err)
			}

			if !cabinetLookup.CabinetExists(cabinetXname) {
				err := fmt.Errorf("unknown cabinet (%s)", cabinetXname)
				panic(err)
			}
		}

		// Verify new hardware
		if _, present := allHardware[hardware.Xname]; present {
			err := fmt.Errorf("found duplicate xname %v", hardware.Xname)
			panic(err)
		}

		allHardware[hardware.Xname] = hardware

		//
		// Build up derived hardware
		//
		if hardware.TypeString == xnametypes.ChassisBMC {
			allHardware[hardware.Xname] = sls_common.NewGenericHardware(hardware.Parent, hardware.Class, nil)
		}

		//
		// Build the MgmtSwitchConnector for the hardware
		//

		mgmtSwtichConnector, err := ccj.BuildSLSMgmtSwitchConnector(hardware, topologyNode, paddle)
		if err != nil {
			panic(err)
		}

		// Ignore empty mgmtSwtichConnectors
		if mgmtSwtichConnector.Xname == "" {
			continue
		}

		if _, present := allHardware[mgmtSwtichConnector.Xname]; present {
			err := fmt.Errorf("found duplicate xname %v", mgmtSwtichConnector.Xname)
			panic(err)
		}

		allHardware[mgmtSwtichConnector.Xname] = mgmtSwtichConnector
	}

	// Generate Cabinet Objects
	for cabinetKind, cabinets := range cabinetLookup {
		for _, cabinet := range cabinets {
			class, err := cabinetKind.Class()
			if err != nil {
				panic(err)
			}

			extraProperties := sls_common.ComptypeCabinet{
				Networks: map[string]map[string]sls_common.CabinetNetworks{}, // TODO this should be outright removed. MEDS and KEA no longer look here
			}

			if cabinetKind.IsModel() {
				extraProperties.Model = string(cabinetKind)
			}

			hardware := sls_common.NewGenericHardware(cabinet, class, extraProperties)

			// Verify new hardware
			if _, present := allHardware[hardware.Xname]; present {
				err := fmt.Errorf("found duplicate xname %v", hardware.Xname)
				panic(err)
			}

			allHardware[hardware.Xname] = hardware
		}
	}

	// Build up and the SLS state
	return sls_common.SLSState{
		Hardware: allHardware,
	}, nil
}

func (te *TopologyEngine) displayHardwareComparisonReport(hardwareRemoved, hardwareAdded, identicalHardware []sls_common.GenericHardware, hardwareWithDifferingValues []sls.GenericHardwarePair) {
	fmt.Println("Identical hardware between current and expected states")
	if len(identicalHardware) == 0 {
		fmt.Println("  None")
	}
	for _, pair := range identicalHardware {
		fmt.Printf("  %s\n", pair.Xname)
	}

	fmt.Println("Common hardware between current and expected states with differing class or extra properties")
	if len(hardwareWithDifferingValues) == 0 {
		fmt.Println("  None")
	}
	for _, pair := range hardwareWithDifferingValues {
		fmt.Printf("  %s\n", pair.Xname)
	}

	fmt.Println("Hardware added to the system")
	if len(hardwareAdded) == 0 {
		fmt.Println("  None")
	}
	for _, hardware := range hardwareAdded {
		hardwareRaw, err := json.Marshal(hardware)
		if err != nil {
			panic(err)
		}

		fmt.Printf("  %s - %s\n", hardware.Xname, string(hardwareRaw))
	}

	fmt.Println()
	fmt.Println("Hardware removed from system")
	if len(hardwareRemoved) == 0 {
		fmt.Println("  None")
	}
	for _, hardware := range hardwareRemoved {
		hardwareRaw, err := json.Marshal(hardware)
		if err != nil {
			panic(err)
		}

		fmt.Printf("  %s - %s\n", hardware.Xname, string(hardwareRaw))
	}
}

func (te *TopologyEngine) determineCabinetNetwork(networkPrefix string, class sls_common.CabinetType) (string, error) {
	var suffix string
	switch class {
	case sls_common.ClassRiver:
		suffix = "_RVR"
	case sls_common.ClassHill:
		fallthrough
	case sls_common.ClassMountain:
		suffix = "_MTN"
	default:
		return "", fmt.Errorf("unknown cabinet class (%s)", class)
	}

	return networkPrefix + suffix, nil
}
