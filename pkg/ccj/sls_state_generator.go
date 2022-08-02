package ccj

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Cray-HPE/cray-site-init/pkg/csi"
	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/Cray-HPE/hms-xname/xnames"
	"github.com/Cray-HPE/hms-xname/xnametypes"
	"github.hpe.com/sjostrand/topology-tool/pkg/configs"
)

// Paddle Vendor to SLS Brand
var vendorBrandMapping = map[string]string{
	"aruba": "Aruba",
	// TODO Dell
	// TODO Mellanox
}

func extractNumber(numberRaw string) (int, error) {
	matches := regexp.MustCompile(`(\d+)`).FindStringSubmatch(numberRaw)

	if len(matches) < 2 {
		return 0, fmt.Errorf("unexpected number of matches %d expected 2", len(matches))
	}

	number, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, err
	}

	return number, nil
}

func BuildExpectedHardwareState(paddle Paddle, cabinetLookup configs.CabinetLookup, applicationNodeConfig csi.SLSGeneratorApplicationNodeConfig) (sls_common.SLSState, error) {
	// Iterate over the paddle file to build of SLS data
	allHardware := map[string]sls_common.GenericHardware{}
	for _, topologyNode := range paddle.Topology {
		// Debug
		// fmt.Println(topologyNode.Architecture, topologyNode.CommonName)

		//
		// Build the SLS hardware representation
		//
		hardware, err := BuildSLSHardware(topologyNode, paddle, cabinetLookup, applicationNodeConfig)
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

func BuildSLSHardware(topologyNode TopologyNode, paddle Paddle, cabinetLookup configs.CabinetLookup, applicationNodeConfig csi.SLSGeneratorApplicationNodeConfig) (sls_common.GenericHardware, error) {
	// TODO use CANU files for lookup
	switch topologyNode.Architecture {
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
	case "river_compute_node":
		fallthrough
	case "river_ncn_node_4_port_gigabyte":
		fallthrough
	case "river_ncn_node_2_port_gigabyte":
		fallthrough
	case "river_ncn_node_2_port":
		fallthrough
	case "river_ncn_node_4_port":
		// All node architecture needs to go through this function
		return buildSLSNode(topologyNode, paddle, applicationNodeConfig)
	case "mountain_compute_leaf": // CDUMgmtSwitch
		if strings.HasPrefix(topologyNode.Location.Rack, "x") {
			// This CDU MgmtSwitch is present in a river cabinet.
			// This is normally seen on newer TDS/Hill cabinet systems
			return buildSLSMgmtHLSwitch(topologyNode)
		} else if strings.HasPrefix(topologyNode.Location.Rack, "cdu") {
			// TODO untested path
			return buildSLSCDUMgmtSwitch(topologyNode)
		}
	case "customer_edge_router":
		fallthrough
	case "spine":
		fallthrough
	case "river_ncn_leaf":
		return buildSLSMgmtHLSwitch(topologyNode)
	case "river_bmc_leaf":
		return buildSLSMgmtSwitch(topologyNode)
	}

	return sls_common.GenericHardware{}, fmt.Errorf("unknown architecture type %s", topologyNode.Architecture)
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

func buildSLSNode(topologyNode TopologyNode, paddle Paddle, applicationNodeConfig csi.SLSGeneratorApplicationNodeConfig) (sls_common.GenericHardware, error) {
	cabinetOrdinal, err := extractNumber(topologyNode.Location.Rack)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract cabinet ordinal due to: %w", err)
	}

	// This rack U is used for single and dual node chassis
	rackUOrdinal, err := extractNumber(topologyNode.Location.Elevation)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract rack U ordinal due to: %w", err)
	}

	chassisOrdinal := 0 // TODO EX2500

	bmcOrdinal := 0 // This is the default for single node chassis

	// Build up the nodes ExtraProperties
	var extraProperties sls_common.ComptypeNode

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
			return sls_common.GenericHardware{}, fmt.Errorf("unable to extract NID from common name (%s) due to: %w", topologyNode.CommonName, err)
		}

		// The CANU common name is different the compute node aliases that are present in SLS
		extraProperties.Aliases = []string{
			fmt.Sprintf("nid%06d", extraProperties.NID),
		}

	} else {
		// Must be an application node
		// Application nodes don't have a NID due to reasons.
		extraProperties.Role = "Application"
	}

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
				return sls_common.GenericHardware{}, fmt.Errorf("unable to find parent topology node with id (%v)", cmcPorts[0].DestNodeID)
			}
		} else {
			return sls_common.GenericHardware{}, fmt.Errorf("unexpected number of 'cmc' ports found (%v) expected 1", len(cmcPorts))
		}

		// This nodes cabinet and the CMC cabinet need to agrees
		if topologyNode.Location.Rack != cmc.Location.Rack {
			return sls_common.GenericHardware{}, fmt.Errorf("parent topology has inconsistent rack location (%v) expected %v", cmc.Location.Rack, topologyNode.Location.Rack)
		}

		// TODO Verify the Parent is either equal rack elevation or 1 below this node
		// As that is the current custom of how these xnames are derived.

		// Override the rack U with the CMC/parent rack U
		rackUOrdinal, err = extractNumber(cmc.Location.Elevation)
		if err != nil {
			return sls_common.GenericHardware{}, fmt.Errorf("unable to extract rack U ordinal from parent topology node due to: %w", err)
		}

		// Calculate the BMC ordinal, which is derived from its NID.
		if extraProperties.Role != "Compute" {
			return sls_common.GenericHardware{}, fmt.Errorf("calculating BMC ordinal for a dense quad node chassis for a non compute node (%v). Is this even supported?", extraProperties.Role)
		}
		if extraProperties.NID == 0 {
			return sls_common.GenericHardware{}, fmt.Errorf("are zero NIDs even supported? I don't think so...")
		}
		bmcOrdinal = ((extraProperties.NID - 1) % 4) + 1
	}

	// TODO Is this ia dual node chassis?
	// Otherwise this is a single node chassis

	xname := xnames.Node{
		Cabinet:       cabinetOrdinal,
		Chassis:       chassisOrdinal,
		ComputeModule: rackUOrdinal,
		NodeBMC:       bmcOrdinal,
		Node:          0, // Assumption: Currently all river hardware that CSM supports BMCs only control one node.
	}

	if extraProperties.Role == "Application" {
		// Merge default Application node prefixes with the user provided prefixes.
		prefixes := []string{}
		prefixes = append(prefixes, applicationNodeConfig.Prefixes...)
		prefixes = append(prefixes, csi.DefaultApplicationNodePrefixes...)

		// Merge default Application node subroles with the user provided subroles. User provided subroles can override the default subroles
		subRoles := map[string]string{}
		for prefix, subRole := range csi.DefaultApplicationNodeSubroles {
			subRoles[prefix] = subRole
		}
		for prefix, subRole := range applicationNodeConfig.PrefixHSMSubroles {
			subRoles[prefix] = subRole
		}

		// Check source to see if it matches any know application node prefix
		for _, prefix := range prefixes {
			if strings.HasPrefix(topologyNode.CommonName, prefix) {
				// Found an application node!
				extraProperties.SubRole = subRoles[prefix]
			}
		}

		// Question: Does it make sense for application nodes to not have a sub-role? It has caused more confision then it has helped.

		// TODO/Question CANU common name or provided aliases via application node config?
		// TODO CANU Paddle has 3 padded zeros for UANs like uan003, but customers may define 2 zero passing like uan03.
		// extraProperties.Aliases = []string{
		// 	topologyNode.CommonName,
		// }
		extraProperties.Aliases = applicationNodeConfig.Aliases[xname.String()]

		if len(extraProperties.Aliases) == 0 {
			return sls_common.GenericHardware{}, fmt.Errorf("application node (%s) has no defined aliases", xname.String())
		}
	}

	return sls_common.NewGenericHardware(xname.String(), sls_common.ClassRiver, extraProperties), nil
}

func buildSLSMgmtSwitch(topologyNode TopologyNode) (sls_common.GenericHardware, error) {
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

	slsBrand, ok := vendorBrandMapping[topologyNode.Vendor]
	if !ok {
		return sls_common.GenericHardware{}, fmt.Errorf("unknown topology node vendor: (%s)", topologyNode.Vendor)
	}

	extraProperties := sls_common.ComptypeMgmtSwitch{
		Brand: slsBrand,
		Model: topologyNode.Model,
		Aliases: []string{
			topologyNode.CommonName,
		},
		// IP4Addr: , // TODO the hms-discovery job and REDS should be using DNS for the HMN IP of the leaf-bmc switch
		SNMPAuthPassword: fmt.Sprintf("vault://hms-creds/%s", xname.String()),
		SNMPAuthProtocol: "MD5",
		SNMPPrivPassword: fmt.Sprintf("vault://hms-creds/%s", xname.String()),
		SNMPPrivProtocol: "DES",
		SNMPUsername:     "testuser", // TODO the authentication data for the switch should be wholy within vault, and not in SLS
	}

	return sls_common.NewGenericHardware(xname.String(), sls_common.ClassRiver, extraProperties), nil
}

func buildSLSMgmtHLSwitch(topologyNode TopologyNode) (sls_common.GenericHardware, error) {
	cabinetOrdinal, err := extractNumber(topologyNode.Location.Rack)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract cabinet ordinal due to: %w", err)
	}

	rackUOrdinal, err := extractNumber(topologyNode.Location.Elevation)
	if err != nil {
		return sls_common.GenericHardware{}, fmt.Errorf("unable to extract rack U ordinal due to: %w", err)
	}

	spaceOrdinal := 1                                              // Defaults to 0 if this is the switch occupies the whole rack U. Not one half of it
	if strings.ToLower(topologyNode.Location.SubLocation) == "l" { // TODO will CANU always captailize it? No
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

	var slsBrand string
	if brand, ok := vendorBrandMapping[topologyNode.Vendor]; ok {
		slsBrand = brand
	} else if topologyNode.Architecture == "customer_edge_router" {
		// TODO This information is missing from the paddle, but is present in SLS via switch_metadata.csv
		slsBrand = "Arista" // TODO HACK right now I think we only support Arista edge routers
	} else {
		return sls_common.GenericHardware{}, fmt.Errorf("unknown topology node vendor: (%s)", topologyNode.Vendor)
	}

	extraProperties := sls_common.ComptypeMgmtHLSwitch{
		Brand: slsBrand,
		Model: topologyNode.Model,
		Aliases: []string{
			topologyNode.CommonName,
		},
		// IP4Addr: , // TODO the hms-discovery job and REDS should be using DNS for the HMN IP of the leaf-bmc switch
	}

	return sls_common.NewGenericHardware(xname.String(), sls_common.ClassRiver, extraProperties), nil

}

func buildSLSCDUMgmtSwitch(topologyNode TopologyNode) (sls_common.GenericHardware, error) {
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

	slsBrand, ok := vendorBrandMapping[topologyNode.Vendor]
	if !ok {
		return sls_common.GenericHardware{}, fmt.Errorf("unknown topology node vendor: (%s)", topologyNode.Vendor)
	}

	extraProperties := sls_common.ComptypeCDUMgmtSwitch{
		Brand: slsBrand,
		Model: topologyNode.Model,
		Aliases: []string{
			topologyNode.CommonName,
		},
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
		fmt.Printf("%s (%s) does not have a connection to the HMN\n", hardware.Xname, topologyNode.CommonName)
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
		// TODO we don't support this for BMCs
		fallthrough
	default:
		return sls_common.GenericHardware{}, fmt.Errorf("unexpected switch vendor (%s)", topologyNode.Vendor)
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
