package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"

	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/Cray-HPE/hms-xname/xnames"
)

// The following structures where taken from CSI (and slight renamed)
// https://github.com/Cray-HPE/cray-site-init/blob/main/pkg/shcd/shcd.go

type Paddle struct {
	Architecture string         `json:"architecture"`
	CanuVersion  string         `json:"canu_version"`
	ShcdFile     string         `json:"shcd_file"`
	UpdatedAt    string         `json:"updated_at"`
	Topology     []TopologyNode `json:"topology"`
}

type TopologyNode struct {
	Architecture string   `json:"architecture"`
	CommonName   string   `json:"common_name"`
	ID           int      `json:"id"`
	Location     Location `json:"location"`
	Model        string   `json:"model"`
	Ports        []Port   `json:"ports"`
	Type         string   `json:"type"`
	Vendor       string   `json:"vendor"`
}

// The Port type defines where things are plugged in
type Port struct {
	DestNodeID int    `json:"destination_node_id"`
	DestPort   int    `json:"destination_port"`
	DestSlot   string `json:"destination_slot"`
	Port       int    `json:"port"`
	Slot       string `json:"slot"`
	Speed      int    `json:"speed"`
}

// The Location type defines where the server physically exists in the datacenter.
type Location struct {
	Elevation string `json:"elevation"`
	Rack      string `json:"rack"`
}

func main() {
	if len(os.Args) != 2 {
		panic("Incorrect number of CLI args provided")
	}

	// Read in the paddle file
	paddleRaw, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}

	var paddle Paddle
	if err := json.Unmarshal(paddleRaw, &paddle); err != nil {
		panic(err)
	}

	// TODO Verify Paddle
	// - Check CANU Version?
	// - Check Architecture against list of supported

	// Iterate over the paddle file to build of SLS data
	allHardware := map[string]sls_common.GenericHardware{}
	for _, topologyNode := range paddle.Topology {
		fmt.Println(topologyNode.Architecture, topologyNode.CommonName)

		hardware, err := buildSLSHardware(topologyNode)
		if err != nil {
			panic(err)
		}

		// Ignore empty hardware
		if hardware.Xname == "" {
			continue
		}

		if _, present := allHardware[hardware.Xname]; present {
			err := fmt.Errorf("found duplicate xname %v", hardware.Xname)
			panic(err)
		}

		allHardware[hardware.Xname] = hardware
	}

	// Build up and the SLS state
	slsState := sls_common.SLSState{
		Hardware: allHardware,
	}

	// Write out the SLS State file
	slsSateRaw, err := json.MarshalIndent(slsState, "", "  ")
	if err != nil {
		panic(err)
	}

	err = ioutil.WriteFile("sls_state.json", slsSateRaw, 0600)
	if err != nil {
		panic(err)
	}
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

func buildSLSHardware(topologyNode TopologyNode) (sls_common.GenericHardware, error) {
	switch topologyNode.Architecture {
	case "subrack":
		return buildSLSCMC(topologyNode.Location)
	case "pdu":
		return buildSLSPDUController(topologyNode.Location)
	case "slingshot_hsn_switch":
		return buildSLSSlingshotHSNSwitch(topologyNode.Location)
	case "river_compute_node":
		// Compute Nodes

		return sls_common.GenericHardware{}, nil
	case "river_ncn_node_2_port":
		fallthrough
	case "river_ncn_node_4_port":
		// NCNs: Management and Application
		return sls_common.GenericHardware{}, nil
	case "spine":
		return sls_common.GenericHardware{}, nil
	case "river_bmc_leaf":
		return sls_common.GenericHardware{}, nil
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
