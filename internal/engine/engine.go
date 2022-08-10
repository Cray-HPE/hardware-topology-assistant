package engine

import (
	"encoding/json"
	"fmt"
	"sort"

	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/Cray-HPE/hms-xname/xnames"
	"github.com/Cray-HPE/hms-xname/xnametypes"
	"github.com/mitchellh/mapstructure"
	"github.hpe.com/sjostrand/topology-tool/pkg/ccj"
	"github.hpe.com/sjostrand/topology-tool/pkg/configs"
	"github.hpe.com/sjostrand/topology-tool/pkg/ipam"
	"github.hpe.com/sjostrand/topology-tool/pkg/sls"
	"gopkg.in/yaml.v2"
)

type TopologyEngine struct {
	Input EngineInput
}

type EngineInput struct {
	Paddle                  ccj.Paddle
	ApplicationNodeMetadata configs.ApplicationNodeMetadataMap

	CurrentSLSState sls_common.SLSState
}

type SubnetChange struct {
	NetworkName string
	Subnet      sls_common.IPV4Subnet
}
type IPReservationChange struct {
	NetworkName   string
	SubnetName    string
	IPReservation sls_common.IPReservation

	// TODO have a better description of what caused the changed

	// This is the hardware object that triggered the change
	// If empty, then this was not changed by hardware
	ChangedByXname string
}

type TopologyChanges struct {
	// The following fields are meant to pushed back into SLS
	HardwareAdded    []sls_common.GenericHardware
	ModifiedNetworks map[string]sls_common.Network

	// The following fields are for book keeping to trigger other events
	SubnetsAdded        []SubnetChange
	IPReservationsAdded []IPReservationChange

	// TODO Add in HSM EthernetEthernetInterface information
	// This is needed if the state IP address range for a network needs to be expanded
	// so we can check to see if the IP has been allocated.
	// These issues need to be recorded, as the subnets DHCP range needs to be expanded.
}

func (te *TopologyEngine) DetermineChanges() (*TopologyChanges, error) {
	//
	// Build the expected hardware state of the system
	//

	// Build the Cabinet lookup structure from the provided CCJ
	cabinetLookup, err := ccj.DetermineCabinetLookup(te.Input.Paddle)
	if err != nil {
		return nil, fmt.Errorf("failed to build the cabinet lookup: %w", err)
	}

	{
		// Debug
		cabinetLookupRaw, err := yaml.Marshal(cabinetLookup)
		if err != nil {
			return nil, err
		}

		fmt.Println("Cabinet lookup:")
		fmt.Println(string(cabinetLookupRaw))
	}

	// Build up a list of current switch alias overrides
	currentSwitchAliases, err := sls.SwitchAliases(te.Input.CurrentSLSState.Hardware)
	if err != nil {
		return nil, fmt.Errorf("failed to extract switch aliases from current SLS hardware state: %w", err)
	}

	// Build up the expected SLS hardware state from the provided CCJ
	expectedSLSState, err := ccj.BuildExpectedHardwareState(te.Input.Paddle, cabinetLookup, te.Input.ApplicationNodeMetadata, currentSwitchAliases)
	if err != nil {
		return nil, fmt.Errorf("failed to build expected SLS hardware state: %w", err)
	}

	// Prune Mountain hardware from current and expected state
	// The initial version of this tool is aimed toward to adding river hardware only, so lets strip
	// mountain hardware from consideration.
	// Also note no need to check the value of error, as its only generated by the filter function
	expectedSLSState.Hardware, _ = sls.FilterHardware(expectedSLSState.Hardware, func(hardware sls_common.GenericHardware) (bool, error) {
		return hardware.Class == sls_common.ClassRiver, nil
	})
	te.Input.CurrentSLSState.Hardware, _ = sls.FilterHardware(te.Input.CurrentSLSState.Hardware, func(hardware sls_common.GenericHardware) (bool, error) {
		return hardware.Class == sls_common.ClassRiver, nil
	})

	// Prune Management NCNs as they need to be
	// The initial version of this tool aims to add non-management NCN river hardware, so lets strip
	// the management NCNs from consideration.
	expectedSLSState.Hardware, err = sls.FilterOutManagementNCNs(expectedSLSState.Hardware)
	if err != nil {
		return nil, fmt.Errorf("failed to filter out management NCNs from expected SLS hardware state: %w", err)
	}
	te.Input.CurrentSLSState.Hardware, err = sls.FilterOutManagementNCNs(te.Input.CurrentSLSState.Hardware)
	if err != nil {
		return nil, fmt.Errorf("failed to filter out management NCNs from current SLS hardware state: %w", err)
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

	// TODO Verify that no hardware was moved, which would appear as a remove and add.
	// TODO Verify all of the new hardware has unique aliases.

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

	// More bookkeeping to keep track of what network items have changed at a more granular level
	subnetsAdded := []SubnetChange{}
	ipReservationsAdded := []IPReservationChange{}

	// First look for any new cabinets, and allocation an subnet for them
	// Note: The hardware being added is sorted by xname so this should be deterministic
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

				fmt.Printf("Allocated subnet %s in network %s for %s\n", subnet.CIDR, networkName, hardware.Xname)
				subnetsAdded = append(subnetsAdded, SubnetChange{
					NetworkName: networkName,
					Subnet:      subnet,
				})

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
	// Note: The hardware being added is sorted by xname so this should be deterministic
	for i, hardware := range hardwareAdded {
		hmsType := hardware.TypeString
		if hmsType == xnametypes.CDUMgmtSwitch || hmsType == xnametypes.MgmtHLSwitch || hmsType == xnametypes.MgmtSwitch {
			// Get the switch's alias
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

			if len(aliases) != 1 {
				return nil, fmt.Errorf("switch (%s) has unexpected number of aliases (%d) expected 1", hardware.Xname, len(aliases))
			}

			// Allocation IP for Switch
			fmt.Printf("%s (%s): Allocating IPs for switch\n", hardware.Xname, aliases[0])

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
				if xname == nil {
					return nil, fmt.Errorf("unable to parse switch xname (%s)", hardware.Xname)
				}

				// Allocate the IP!
				ipReservation, err := ipam.AllocateIP(slsSubnet, xname, aliases[0])
				if err != nil {
					return nil, fmt.Errorf("unable to allocate IP for switch (%s) in network (%s): %w", xname.String(), networkName, err)
				}

				fmt.Printf("%s (%s): Allocated IP %s in subnet network_hardware in network %s\n", hardware.Xname, aliases[0], ipReservation.IPAddress.String(), networkName)
				ipReservationsAdded = append(ipReservationsAdded, IPReservationChange{
					NetworkName:    networkName,
					SubnetName:     "network_hardware",
					IPReservation:  ipReservation,
					ChangedByXname: hardware.Xname,
				})

				// Push in the network IP Reservation into the subnet
				slsSubnet.IPReservations = append(slsSubnet.IPReservations, ipReservation)
				networkExtraProperties.Subnets[slsSubnetIndex] = slsSubnet
				modifiedNetworks[networkName] = true

				// Update the hardware object to have the HMN IP, which is required for the hms-discovery and REDS to function
				// In the future this should be phased out.
				if hmsType == xnametypes.MgmtSwitch && networkName == "HMN" {
					extraProperties, ok := hardware.ExtraPropertiesRaw.(sls_common.ComptypeMgmtSwitch)
					if !ok {
						return nil, fmt.Errorf("unable to get extra properties for switch (%s)", hardware.Xname)
					}

					extraProperties.IP4Addr = ipReservation.IPAddress.String()

					// Push the updated extra properties back into the list of new hardware
					hardware.ExtraPropertiesRaw = extraProperties
					hardwareAdded[i] = hardware
				}

				// I know of nothing that uses this IP for MgmtHLSwitches, but it is how CSI creates this hardware object,
				// so we should add it
				if hmsType == xnametypes.MgmtHLSwitch && networkName == "HMN" {
					extraProperties, ok := hardware.ExtraPropertiesRaw.(sls_common.ComptypeMgmtHLSwitch)
					if !ok {
						return nil, fmt.Errorf("unable to get extra properties for switch (%s)", hardware.Xname)
					}

					extraProperties.IP4Addr = ipReservation.IPAddress.String()

					// Push the updated extra properties back into the list of new hardware
					hardware.ExtraPropertiesRaw = extraProperties
					hardwareAdded[i] = hardware
				}
			}

		}
	}

	// Allocate UAN IPs on the CAN or CHN
	// UH-OH the CAN/CHN range tightly packs the Static and DHCP IP address ranges right next to each other.
	// So if we need to allocate an UAN IP on the CHN, then the Static IP address range needs to be expanded.
	// Since this in on the CAN/CHN no nodes will be using these IPs for booting over DVS (either NMN or HSM)
	// This will make adjusting the DHCP range nicer.
	type uanInfo struct {
		xname string
		alias string
	}
	var uans []uanInfo
	for _, hardware := range hardwareAdded {
		if hardware.TypeString != xnametypes.Node {
			continue
		}

		var extraProperties sls_common.ComptypeNode
		if err := mapstructure.Decode(hardware.ExtraPropertiesRaw, &extraProperties); err != nil {
			return nil, fmt.Errorf("unable to decode extra properties for (%s)", hardware.Xname)
		}

		if extraProperties.Role == "Application" && extraProperties.SubRole == "UAN" {
			if len(extraProperties.Aliases) == 0 {
				return nil, fmt.Errorf("no aliases defined for (%s)", hardware.Xname)
			}

			uans = append(uans, uanInfo{
				xname: hardware.Xname,
				alias: extraProperties.Aliases[0],
			})
		}
	}

	// Sort the UANs to add them increasing order to be deterministic
	sort.SliceStable(uans, func(i, j int) bool {
		return uans[i].alias < uans[j].alias
	})

	if len(uans) != 0 {
		// Check to see if the Static IP address range in the CAN/CHN networks needs to be expanded
		// to accommodate the new UANs.
		for _, networkName := range []string{"CAN", "CHN"} {
			// Only allocate an IP for the UAN if the network exists
			networkExtraProperties, present := networkExtraProperties[networkName]
			if !present {
				continue
			}
			fmt.Printf("Checking to see if the static IP address range for the bootstrap_dhcp subnet in %s has enough room for added UAN(s).\n", networkName)

			// Retrieve the subnet
			slsSubnet, slsSubnetIndex, err := networkExtraProperties.LookupSubnet("bootstrap_dhcp")
			if !present {
				return nil, fmt.Errorf("unable to find subnet in (%s) network: %w", networkName, err)
			}

			freeIPCount, err := ipam.FreeIPsInStaticRange(slsSubnet)
			if err != nil {
				return nil, fmt.Errorf("unable to determine the number of free IPs in the Static IP range in bootstrap_dhcp subnet in (%s) network: %w", networkName, err)
			}

			var expandStaticRangeBy uint32
			if freeIPCount < uint32(len(uans)) {
				expandStaticRangeBy = uint32(len(uans)) - freeIPCount
				fmt.Printf("The bootstrap_dhcp subnet in %s network has %d IP addresses available, will be expanded by %d hosts.\n", networkName, freeIPCount, expandStaticRangeBy)

				// Okay, lets see if we can expand the subnet by the number of UANs being added to the system
				if err := ipam.ExpandSubnetStaticRange(&slsSubnet, uint32(len(uans))); err != nil {
					return nil, fmt.Errorf("unable to expand the static IP address range in the bootstrap_dhcp subnet in (%s) network: %w", networkName, err)
				}
				fmt.Printf("The bootstrap_dhcp subnet in %s network has been expanded by %d IP addresses,\n", networkName, expandStaticRangeBy)

				// Update the subnet with the new DHCP range
				networkExtraProperties.Subnets[slsSubnetIndex] = slsSubnet
				modifiedNetworks[networkName] = true
			} else {
				fmt.Printf("The bootstrap_dhcp subnet in %s network has %d IP addresses available.\n", networkName, freeIPCount)
			}

		}
	}

	// Allocate IP addresses
	for _, uan := range uans {
		fmt.Printf("%s (%s): Allocating IPs For UAN\n", uan.xname, uan.alias)

		for _, networkName := range []string{"CAN", "CHN"} {
			// Only allocate an IP for the UAN if the network exists
			networkExtraProperties, present := networkExtraProperties[networkName]
			if !present {
				continue
			}

			// Debug
			//fmt.Printf("%s (%s): Allocating IP on the %s network\n", uan.xname, uan.alias, networkName)

			// Retrieve the subnet
			slsSubnet, slsSubnetIndex, err := networkExtraProperties.LookupSubnet("bootstrap_dhcp")
			if !present {
				return nil, fmt.Errorf("unable to find subnet in (%s) network: %w", networkName, err)
			}

			// Parse the xname
			xname := xnames.FromString(uan.xname)
			if xname == nil {
				return nil, fmt.Errorf("unable to parse UAN xname (%s)", uan.xname)
			}

			ipReservation, err := ipam.AllocateIP(slsSubnet, xname, uan.alias)
			if err != nil {
				return nil, fmt.Errorf("unable to allocate IP for UAN (%s) in network (%s): %w", xname.String(), networkName, err)
			}
			ipReservationsAdded = append(ipReservationsAdded, IPReservationChange{
				NetworkName:    networkName,
				SubnetName:     "bootstrap_dhcp",
				IPReservation:  ipReservation,
				ChangedByXname: uan.xname,
			})

			fmt.Printf("%s (%s): Allocated IP %s on the %s network\n", uan.xname, uan.alias, ipReservation.IPAddress, networkName)

			// Push in the network IP Reservation into the subnet
			slsSubnet.IPReservations = append(slsSubnet.IPReservations, ipReservation)
			networkExtraProperties.Subnets[slsSubnetIndex] = slsSubnet
			modifiedNetworks[networkName] = true
		}

	}

	// Filter NetworkExtraProperties to include only the modified networks
	modifiedNetworksSet := map[string]sls_common.Network{}
	for networkName, networkExtraProperties := range networkExtraProperties {
		if !modifiedNetworks[networkName] {
			continue
		}

		// Merge extra properties with the top level network with SLS
		slsNetwork := te.Input.CurrentSLSState.Networks[networkName]
		slsNetwork.ExtraPropertiesRaw = networkExtraProperties

		// TODO update vlan range.

		modifiedNetworksSet[networkName] = slsNetwork
	}

	return &TopologyChanges{
		HardwareAdded:    hardwareAdded,
		ModifiedNetworks: modifiedNetworksSet,

		SubnetsAdded:        subnetsAdded,
		IPReservationsAdded: ipReservationsAdded,
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

		// Expected Hardware json
		pair.HardwareA.LastUpdated = 0
		pair.HardwareA.LastUpdatedTime = ""
		hardwareRaw, err := json.Marshal(pair.HardwareA)
		if err != nil {
			panic(err)
		}
		fmt.Printf("  - Expected: %s\n", string(hardwareRaw))

		// Actual Hardware json
		pair.HardwareB.LastUpdated = 0
		pair.HardwareB.LastUpdatedTime = ""
		hardwareRaw, err = json.Marshal(pair.HardwareB)
		if err != nil {
			panic(err)
		}
		fmt.Printf("  - Actual:   %s\n", string(hardwareRaw))
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

		fmt.Printf("  %s\n", hardware.Xname)
		fmt.Printf("  - %s\n", string(hardwareRaw))
	}

	fmt.Println("Hardware removed from system")
	if len(hardwareRemoved) == 0 {
		fmt.Println("  None")
	}
	for _, hardware := range hardwareRemoved {
		hardwareRaw, err := json.Marshal(hardware)
		if err != nil {
			panic(err)
		}

		fmt.Printf("  %s\n", hardware.Xname)
		fmt.Printf("  - %s\n", string(hardwareRaw))
	}

	fmt.Println()
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
