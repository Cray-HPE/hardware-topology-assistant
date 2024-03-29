// MIT License
//
// (C) Copyright 2022-2023 Hewlett Packard Enterprise Development LP
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.

package ccj

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/Cray-HPE/cray-site-init/pkg/csi"
	"github.com/Cray-HPE/hardware-topology-assistant/pkg/configs"
	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/Cray-HPE/hms-xname/xnames"
	"github.com/Cray-HPE/hms-xname/xnametypes"
)

// Paddle Vendor to SLS Brand
var vendorBrandMapping = map[string]string{
	"aruba":    "Aruba",
	"dell":     "Dell",
	"mellanox": "Mellanox",
}

func extractNumber(numberRaw string) (int, error) {
	matches := regexp.MustCompile(`(\d+)`).FindStringSubmatch(strings.ToLower(numberRaw))

	if len(matches) < 2 {
		return 0, fmt.Errorf("unexpected number of matches %d expected 2", len(matches))
	}

	number, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, err
	}

	return number, nil
}

func BuildExpectedHardwareState(paddle Paddle, cabinetLookup configs.CabinetLookup, applicationNodeMetadata configs.ApplicationNodeMetadataMap, switchAliasesOverrides map[string][]string, ignoreUnknownCANUHardwareArchitectures bool) (sls_common.SLSState, error) {
	// Iterate over the paddle file to build of SLS data
	allHardware := map[string]sls_common.GenericHardware{}
	for _, topologyNode := range paddle.Topology {
		//
		// Build the SLS hardware representation
		//
		hardware, err := BuildSLSHardware(topologyNode, paddle, cabinetLookup, applicationNodeMetadata, switchAliasesOverrides)
		if err != nil && ignoreUnknownCANUHardwareArchitectures && strings.Contains(err.Error(), "unknown architecture type") {
			log.Printf("WARNING %s", err.Error())
		} else if err != nil {
			log.Fatalf("Error %v", err)
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

		mgmtSwtichConnector, err := BuildSLSMgmtSwitchConnector(hardware, topologyNode, paddle)
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
				Networks: map[string]map[string]sls_common.CabinetNetworks{}, // TODO this should be outright removed. MEDS and KEA no longer look here for network info, but MEDS still needs this key to exist.
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

func BuildSLSHardware(topologyNode TopologyNode, paddle Paddle, cabinetLookup configs.CabinetLookup, applicationNodeMetadata configs.ApplicationNodeMetadataMap, switchAliasesOverrides map[string][]string) (sls_common.GenericHardware, error) {
	// TODO use CANU files for lookup
	// ALso look at using type
	switch topologyNode.Architecture {
	case "kvm":
		// TODO SLS does not know anything about KVM, because HMS software doesn't support them.
		fallthrough
	case "cec":
		// TODO SLS does not know anything about CEC, because HMS software doesn't support them.
		return sls_common.GenericHardware{}, nil
	case "cmm":
		return buildSLSChassisBMC(topologyNode.Location, cabinetLookup)
	case "subrack":
		return buildSLSCMC(topologyNode.Location)
	case "pdu":
		return buildSLSPDUController(topologyNode.Location)
	case "slingshot_hsn_switch":
		return buildSLSSlingshotHSNSwitch(topologyNode.Location)
	case "mountain_compute_leaf": // CDUMgmtSwitch
		if strings.HasPrefix(topologyNode.Location.Rack, "x") {
			// This CDU MgmtSwitch is present in a river cabinet.
			// This is normally seen on newer TDS/Hill cabinet systems
			return buildSLSMgmtHLSwitch(topologyNode, switchAliasesOverrides)
		} else {
			// Otherwise the switch is in a CDU cabinet
			return buildSLSCDUMgmtSwitch(topologyNode, switchAliasesOverrides)
		}
	case "customer_edge_router":
		fallthrough
	case "spine":
		fallthrough
	case "river_ncn_leaf":
		return buildSLSMgmtHLSwitch(topologyNode, switchAliasesOverrides)
	case "river_bmc_leaf":
		return buildSLSMgmtSwitch(topologyNode, switchAliasesOverrides)
	default:
		// There are a lot of architecture types that can be a node, but for SLS we just need to know that it is a server
		// of some sort.
		if topologyNode.Type == "node" || topologyNode.Type == "server" {
			// All node architecture needs to go through this function
			return buildSLSNode(topologyNode, paddle, applicationNodeMetadata)
		}
	}

	return sls_common.GenericHardware{}, fmt.Errorf("unknown architecture type %s for CANU common name %s", topologyNode.Architecture, topologyNode.CommonName)
}

func buildSLSPDUController(location Location) (sls_common.GenericHardware, error) {
	cabinetOrdinal, err := extractNumber(location.Rack)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract cabinet ordinal due to: %w", err)
	}

	pduOrdinal, err := extractNumber(location.Elevation)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract pdu ordinal due to: %w", err)
	}

	xname := xnames.CabinetPDUController{
		Cabinet:              cabinetOrdinal,
		CabinetPDUController: pduOrdinal,
	}

	return sls_common.NewGenericHardware(xname.String(), sls_common.ClassRiver, nil), nil
}

func buildSLSSlingshotHSNSwitch(location Location) (sls_common.GenericHardware, error) {
	cabinetOrdinal, err := extractNumber(location.Rack)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract cabinet ordinal due to: %w", err)
	}

	rackUOrdinal, err := extractNumber(location.Elevation)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract rack U ordinal due to: %w", err)
	}

	xname := xnames.RouterBMC{
		Cabinet:      cabinetOrdinal,
		Chassis:      0, // TODO EX2500
		RouterModule: rackUOrdinal,
		RouterBMC:    0,
	}

	return sls_common.NewGenericHardware(xname.String(), sls_common.ClassRiver, sls_common.ComptypeRtrBmc{
		Username: fmt.Sprintf("vault://hms-creds/%s", xname.String()),
		Password: fmt.Sprintf("vault://hms-creds/%s", xname.String()),
	}), nil
}

func buildSLSCMC(location Location) (sls_common.GenericHardware, error) {
	// TODO what should be done if if the CMC does not have a bmc connection? Ie the Intel CMC that doesn't really exist
	// Right now we are emulating the current behavior of CSI, where the fake CMC exists in SLS and no MgmtSwitchConnector exists.

	cabinetOrdinal, err := extractNumber(location.Rack)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract cabinet ordinal due to: %w", err)
	}

	rackUOrdinal, err := extractNumber(location.Elevation)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract rack U ordinal due to: %w", err)
	}

	xname := xnames.NodeBMC{
		Cabinet:       cabinetOrdinal,
		Chassis:       0, // TODO EX2500
		ComputeModule: rackUOrdinal,
		NodeBMC:       999, // Gigabyte CMCs get this
	}

	return sls_common.NewGenericHardware(xname.String(), sls_common.ClassRiver, nil), nil
}

// BuildNodeExtraProperties will attempt to build up all of the known extra properties form a Node present in a CCJ.
// Limiitations the following information is not populated:
// - Management NCN NID
// - Application Node Subrole and Alias
func BuildNodeExtraProperties(topologyNode TopologyNode) (extraProperties sls_common.ComptypeNode, err error) {
	if topologyNode.Type != "server" && topologyNode.Type != "node" {
		return sls_common.ComptypeNode{}, fmt.Errorf("unexpected topology node type (%s) expected (server or node)", topologyNode.Type)
	}

	// Now to make Sean L sad
	// TODO NCNs need their NID, which is automatically assigned serially via CSI.
	if strings.HasPrefix(topologyNode.CommonName, "ncn-m") {
		extraProperties.Role = "Management"
		extraProperties.SubRole = "Master"
		extraProperties.Aliases = []string{topologyNode.CommonName}
	} else if strings.HasPrefix(topologyNode.CommonName, "ncn-w") {
		extraProperties.Role = "Management"
		extraProperties.SubRole = "Worker"
		extraProperties.Aliases = []string{topologyNode.CommonName}
	} else if strings.HasPrefix(topologyNode.CommonName, "ncn-s") {
		extraProperties.Role = "Management"
		extraProperties.SubRole = "Storage"
		extraProperties.Aliases = []string{topologyNode.CommonName}
	} else if strings.HasPrefix(topologyNode.CommonName, "cn") {
		extraProperties.Role = "Compute"
		extraProperties.NID, err = extractNumber(topologyNode.CommonName)
		if err != nil {
			return sls_common.ComptypeNode{}, fmt.Errorf("unable to extract NID from common name (%s) due to: %w", topologyNode.CommonName, err)
		}

		// The CANU common name is different the compute node aliases that are present in SLS
		extraProperties.Aliases = []string{
			fmt.Sprintf("nid%06d", extraProperties.NID),
		}

	} else {
		// Must be an application node
		// Application nodes don't have a NID due to reasons.
		// The applications subrole gets filled in later
		extraProperties.Role = "Application"
	}

	return extraProperties, nil
}

func BuildNodeXname(topologyNode TopologyNode, paddle Paddle, extraProperties sls_common.ComptypeNode) (xnames.Node, error) {
	if topologyNode.Type != "server" && topologyNode.Type != "node" {
		return xnames.Node{}, fmt.Errorf("unexpected topology node type (%s) expected (server or node)", topologyNode.Type)
	}

	cabinetOrdinal, err := extractNumber(topologyNode.Location.Rack)
	if err != nil {
		return xnames.Node{}, fmt.Errorf("unable to extract cabinet ordinal due to: %w", err)
	}

	// This rack U is used for single and dual node chassis
	rackUOrdinal, err := extractNumber(topologyNode.Location.Elevation)
	if err != nil {
		return xnames.Node{}, fmt.Errorf("unable to extract rack U ordinal due to: %w", err)
	}

	chassisOrdinal := 0 // TODO EX2500

	bmcOrdinal := 0 // This is the default for single node chassis

	// Determine the BMC ordinal and override the rack U if needed
	// Is this an dense quad node chassis?
	if topologyNode.Location.Parent != "" {
		// TODO there is a bug in CANU where the Parent location is not the common name
		// of the BMC. So we have to resort to looking for the CMC connection and looking for the CMC that way
		//
		// The Parent field has SubRack-002-CMC
		// The common field is SubRack002-CMC
		//
		// // Retrieve the parent node
		// cmc, ok := paddle.FindCommonName(topologyNode.Location.Parent)
		// if !ok {
		// 	return sls_common.GenericHardware{}, fmt.Errorf("unable to find parent topology node with common name (%v)", topologyNode.Location.Parent)
		// }
		var cmc TopologyNode
		if cmcPorts := topologyNode.FindPorts("cmc"); len(cmcPorts) == 1 {
			var ok bool
			cmc, ok = paddle.FindNodeByID(cmcPorts[0].DestNodeID)
			if !ok {
				return xnames.Node{}, fmt.Errorf("unable to find parent topology node with id (%v)", cmcPorts[0].DestNodeID)
			}
		} else {
			return xnames.Node{}, fmt.Errorf("unexpected number of 'cmc' ports found (%v) expected 1", len(cmcPorts))
		}

		// This nodes cabinet and the CMC cabinet need to agrees
		if topologyNode.Location.Rack != cmc.Location.Rack {
			return xnames.Node{}, fmt.Errorf("parent topology has inconsistent rack location (%v) expected %v", cmc.Location.Rack, topologyNode.Location.Rack)
		}

		// TODO Verify the Parent is either equal rack elevation or 1 below this node
		// As that is the current custom of how these xnames are derived.

		// Override the rack U with the CMC/parent rack U
		rackUOrdinal, err = extractNumber(cmc.Location.Elevation)
		if err != nil {
			return xnames.Node{}, fmt.Errorf("unable to extract rack U ordinal from parent topology node due to: %w", err)
		}

		// Calculate the BMC ordinal, which is derived from its NID.
		if extraProperties.Role != "Compute" {
			return xnames.Node{}, fmt.Errorf("calculating BMC ordinal for a dense quad node chassis for a non compute node (%v) which is currently not supported", extraProperties.Role)
		}
		if extraProperties.NID == 0 {
			return xnames.Node{}, fmt.Errorf("found compute node with a NID of 0")
		}
		bmcOrdinal = ((extraProperties.NID - 1) % 4) + 1
	} else if strings.ToLower(topologyNode.Location.SubLocation) == "l" {
		// This is the left side node in a dual node chassis
		bmcOrdinal = 1
	} else if strings.ToLower(topologyNode.Location.SubLocation) == "r" {
		// This is the left side node in a dual node chassis
		bmcOrdinal = 2
	}

	xname := xnames.Node{
		Cabinet:       cabinetOrdinal,
		Chassis:       chassisOrdinal,
		ComputeModule: rackUOrdinal,
		NodeBMC:       bmcOrdinal,
		Node:          0, // Assumption: Currently all river hardware that CSM supports BMCs only control one node.
	}

	return xname, nil
}

func buildSLSNode(topologyNode TopologyNode, paddle Paddle, applicationNodeMetadata configs.ApplicationNodeMetadataMap) (sls_common.GenericHardware, error) {
	// Build up the nodes ExtraProperties
	extraProperties, err := BuildNodeExtraProperties(topologyNode)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to build node extra properties: %w", err)
	}

	// Build the xname!
	xname, err := BuildNodeXname(topologyNode, paddle, extraProperties)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to build node xname: %w", err)
	}

	// Now we need to deal with application node specific stuff.
	if extraProperties.Role == "Application" {
		// Question: Does it make sense for application nodes to not have a sub-role? It has caused more confision then it has helped.

		// TODO/Question CANU common name or provided aliases via application node config?
		// TODO CANU Paddle has 3 padded zeros for UANs like uan003, but customers may define 2 zero passing like uan03.
		// extraProperties.Aliases = []string{
		// 	topologyNode.CommonName,
		// }
		metadata, ok := applicationNodeMetadata[xname.String()]
		if !ok {
			return sls_common.GenericHardware{}, fmt.Errorf("unable to find node xname (%s) in the application node metadata map", xname.String())
		}

		extraProperties.SubRole = metadata.SubRole
		extraProperties.Aliases = metadata.Aliases

		if len(extraProperties.Aliases) == 0 {
			return sls_common.GenericHardware{}, fmt.Errorf("application node (%s) has no defined aliases", xname.String())
		}
	}

	return sls_common.NewGenericHardware(xname.String(), sls_common.ClassRiver, extraProperties), nil
}

func buildSLSMgmtSwitch(topologyNode TopologyNode, switchAliasesOverrides map[string][]string) (sls_common.GenericHardware, error) {
	cabinetOrdinal, err := extractNumber(topologyNode.Location.Rack)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract cabinet ordinal due to: %w", err)
	}

	rackUOrdinal, err := extractNumber(topologyNode.Location.Elevation)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract rack U ordinal due to: %w", err)
	}

	xname := xnames.MgmtSwitch{
		Cabinet:    cabinetOrdinal,
		Chassis:    0, // TODO EX2500
		MgmtSwitch: rackUOrdinal,
	}

	// Determine the switch branch
	slsBrand, ok := vendorBrandMapping[topologyNode.Vendor]
	if !ok {
		return sls_common.GenericHardware{}, fmt.Errorf("unknown topology node vendor: (%s)", topologyNode.Vendor)
	}

	// Determine the switch alias, by default use the one from the CCJ
	aliases := []string{topologyNode.CommonName}
	if aliasesOverride, ok := switchAliasesOverrides[xname.String()]; ok {
		// There is a chance that the switch aliases in SLS do not match so lets use the override
		aliases = aliasesOverride
	}

	// Build up the extra properties!
	extraProperties := sls_common.ComptypeMgmtSwitch{
		Brand:   slsBrand,
		Model:   topologyNode.Model,
		Aliases: aliases,
		// IP4Addr: , // TODO the hms-discovery job and REDS should be using DNS for the HMN IP of the leaf-bmc switch
		SNMPAuthPassword: fmt.Sprintf("vault://hms-creds/%s", xname.String()),
		SNMPAuthProtocol: "MD5",
		SNMPPrivPassword: fmt.Sprintf("vault://hms-creds/%s", xname.String()),
		SNMPPrivProtocol: "DES",
		SNMPUsername:     "testuser", // TODO the authentication data for the switch should be wholy within vault, and not in SLS
	}

	return sls_common.NewGenericHardware(xname.String(), sls_common.ClassRiver, extraProperties), nil
}

func buildSLSMgmtHLSwitch(topologyNode TopologyNode, switchAliasesOverrides map[string][]string) (sls_common.GenericHardware, error) {
	cabinetOrdinal, err := extractNumber(topologyNode.Location.Rack)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract cabinet ordinal due to: %w", err)
	}

	rackUOrdinal, err := extractNumber(topologyNode.Location.Elevation)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract rack U ordinal due to: %w", err)
	}

	spaceOrdinal := 1 // Defaults to 1 if this is the switch occupies the whole rack U. Not one half of it
	if strings.ToLower(topologyNode.Location.SubLocation) == "l" {
		spaceOrdinal = 1
	} else if strings.ToLower(topologyNode.Location.SubLocation) == "r" {
		spaceOrdinal = 2
	}

	xname := xnames.MgmtHLSwitch{
		Cabinet:               cabinetOrdinal,
		Chassis:               0, // TODO EX2500
		MgmtHLSwitchEnclosure: rackUOrdinal,
		MgmtHLSwitch:          spaceOrdinal,
	}

	// Determine the switch branch
	var slsBrand string
	if brand, ok := vendorBrandMapping[topologyNode.Vendor]; ok {
		slsBrand = brand
	} else if topologyNode.Architecture == "customer_edge_router" {
		// TODO This information is missing from the paddle, but is present in SLS via switch_metadata.csv
		slsBrand = "Arista"     // TODO HACK right now I think we only support Arista edge routers
		topologyNode.Model = "" // TODO there is no data source for this, but this is a nice to have field inside of SLS.
	} else {
		return sls_common.GenericHardware{}, fmt.Errorf("unknown topology node vendor: (%s)", topologyNode.Vendor)
	}

	// Determine the switch alias, by default use the one from the CCJ
	aliases := []string{topologyNode.CommonName}
	if aliasesOverride, ok := switchAliasesOverrides[xname.String()]; ok {
		// There is a chance that the switch aliases in SLS do not match so lets use the override
		aliases = aliasesOverride
	}

	// Build up the extra properties!
	extraProperties := sls_common.ComptypeMgmtHLSwitch{
		Brand:   slsBrand,
		Model:   topologyNode.Model,
		Aliases: aliases,
		// IP4Addr: , // TODO the hms-discovery job and REDS should be using DNS for the HMN IP of the leaf-bmc switch
	}

	return sls_common.NewGenericHardware(xname.String(), sls_common.ClassRiver, extraProperties), nil

}

func buildSLSCDUMgmtSwitch(topologyNode TopologyNode, switchAliasesOverrides map[string][]string) (sls_common.GenericHardware, error) {
	cduOrdinal, err := extractNumber(topologyNode.Location.Rack)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract cabinet ordinal due to: %w", err)
	}

	rackUOrdinal, err := extractNumber(topologyNode.Location.Elevation)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract rack U ordinal due to: %w", err)
	}

	xname := xnames.CDUMgmtSwitch{
		CDU:           cduOrdinal,
		CDUMgmtSwitch: rackUOrdinal,
	}

	// Determine the switch branch
	slsBrand, ok := vendorBrandMapping[topologyNode.Vendor]
	if !ok {
		return sls_common.GenericHardware{}, fmt.Errorf("unknown topology node vendor: (%s)", topologyNode.Vendor)
	}

	// Determine the switch alias, by default use the one from the CCJ
	aliases := []string{topologyNode.CommonName}
	if aliasesOverride, ok := switchAliasesOverrides[xname.String()]; ok {
		// There is a chance that the switch aliases in SLS do not match so lets use the override
		aliases = aliasesOverride
	}

	// Build up the extra properties!
	extraProperties := sls_common.ComptypeCDUMgmtSwitch{
		Brand:   slsBrand,
		Model:   topologyNode.Model,
		Aliases: aliases,
	}

	return sls_common.NewGenericHardware(xname.String(), sls_common.ClassMountain, extraProperties), nil
}

func BuildSLSMgmtSwitchConnector(hardware sls_common.GenericHardware, topologyNode TopologyNode, paddle Paddle) (sls_common.GenericHardware, error) {
	hmsTypesToIgnore := map[xnametypes.HMSType]bool{
		xnametypes.MgmtHLSwitch:  true,
		xnametypes.MgmtSwitch:    true,
		xnametypes.CDUMgmtSwitch: true,
	}
	if hmsTypesToIgnore[xnametypes.GetHMSType(hardware.Xname)] || hardware.Class != sls_common.ClassRiver {
		return sls_common.GenericHardware{}, nil
	}

	//
	// Determine the xname of the device that this MgmtSwitchConnector will connect to
	//
	var destinationXname string
	if xnametypes.IsHMSTypeController(hardware.TypeString) {
		// This this type *IS* the BMC or PDU, then don't use the parent, use the xname.
		destinationXname = hardware.Xname
	} else {
		destinationXname = hardware.Parent
	}

	//
	// Figure out what switch port the BMC/Controller that is connected to the HMN
	//
	slot := "bmc" // By default lets assume bmc.
	if topologyNode.Architecture == "slingshot_hsn_switch" {
		slot = "mgmt"
	}

	destinationPorts := topologyNode.FindPorts(slot)
	if len(destinationPorts) == 0 {
		log.Printf("%s (%s) does not have a connection to the HMN\n", hardware.Xname, topologyNode.CommonName)
		return sls_common.GenericHardware{}, nil
	} else if len(destinationPorts) != 1 {
		return sls_common.GenericHardware{}, fmt.Errorf("unexpected number of '%s' ports found (%v) expected 1", slot, len(destinationPorts))
	}
	destinationPort := destinationPorts[0]

	destinationTopologyNode, ok := paddle.FindNodeByID(destinationPort.DestNodeID)
	if !ok {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to find destination topology node referenced by port with id (%v)", destinationPort.DestNodeID)
	}

	//
	// Determine the xname of the MgmtSwitch
	//
	// TODO the following could be reused, as it was copied from buildSLSMgmtSwitch, and return a xnames.MgmtSwitch struct
	cabinetOrdinal, err := extractNumber(destinationTopologyNode.Location.Rack)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract cabinet ordinal due to: %w", err)
	}

	rackUOrdinal, err := extractNumber(destinationTopologyNode.Location.Elevation)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract rack U ordinal due to: %w", err)
	}

	mgmtSwitchXname := xnames.MgmtSwitch{
		Cabinet:    cabinetOrdinal,
		Chassis:    0, // TODO EX2500
		MgmtSwitch: rackUOrdinal,
	}

	//
	// Determine the xname of the connector
	//
	xname := mgmtSwitchXname.MgmtSwitchConnector(destinationPort.DestPort)

	//
	// Build the SLS object
	//

	// Calculate the vendor name for the ethernet interfaces
	// Dell switches use: ethernet1/1/1
	// Aruba switches use: 1/1/1
	var vendorName string
	switch destinationTopologyNode.Vendor {
	case "dell":
		vendorName = fmt.Sprintf("ethernet1/1/%d", destinationPort.DestPort)
	case "aruba":
		vendorName = fmt.Sprintf("1/1/%d", destinationPort.DestPort)
	case "mellanox":
		// TODO we don't support this switch for BMC connections
		fallthrough
	default:
		return sls_common.GenericHardware{}, fmt.Errorf("unexpected switch vendor (%s)", destinationTopologyNode.Vendor)
	}
	return sls_common.NewGenericHardware(xname.String(), sls_common.ClassRiver, sls_common.ComptypeMgmtSwitchConnector{
		NodeNics: []string{
			destinationXname,
		},
		VendorName: vendorName,
	}), nil
}

func buildSLSChassisBMC(location Location, cl configs.CabinetLookup) (sls_common.GenericHardware, error) {
	cabinetOrdinal, err := extractNumber(location.Rack)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract cabinet ordinal due to: %w", err)
	}

	chassisOrdinal, err := extractNumber(location.Elevation)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract chassis ordinal due to: %w", err)
	}

	xname := xnames.ChassisBMC{
		Cabinet:    cabinetOrdinal,
		Chassis:    chassisOrdinal,
		ChassisBMC: 0,
	}

	class, err := cl.CabinetClass(xname.Parent().Parent().String())
	if err != nil {
		return sls_common.GenericHardware{}, err
	}

	return sls_common.NewGenericHardware(xname.String(), class, nil), nil
}

// TODO The following was taking from CSI, and has the broken NID allocation logic.
// Also needs a source for the starting nid for the chassis.
//
// func buildLiquidCooledNodeHardware(chassis sls_common.GenericHardware) ([]sls_common.GenericHardware, error) {
// 	for slotOrdinal := 0; slotOrdinal < 8; slotOrdinal++ {
// 		for bmcOrdinal := 0; bmcOrdinal < 2; bmcOrdinal++ {
// 			for nodeOrdinal := 0; nodeOrdinal < 2; nodeOrdinal++ {
// 				// Construct the xname for the node
// 				nodeXname := chassisXname.ComputeModule(slotOrdinal).NodeBMC(bmcOrdinal).Node(nodeOrdinal)
//
// 				node := g.buildSLSHardware(nodeXname, cabinetTemplate.Class, sls_common.ComptypeNode{
// 					NID:     g.currentMountainNID,
// 					Role:    "Compute",
// 					Aliases: []string{fmt.Sprintf("nid%06d", g.currentMountainNID)},
// 				})
//
// 				hardware = append(hardware, node)
//
// 				g.currentMountainNID++
// 			}
// 		}
// 	}
// }
