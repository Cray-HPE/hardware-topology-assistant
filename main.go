package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/Cray-HPE/cray-site-init/pkg/csi"
	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/Cray-HPE/hms-xname/xnames"
	"github.com/Cray-HPE/hms-xname/xnametypes"
	"gopkg.in/yaml.v2"
)

// TODO liquid-cooled nodes are missing
// TODO management NCNs are missing NIDs
// TODO network hardware objects are missing IPs (which is a hold over)

// TODO replace with the actual cabinets.yaml
// TODO Normalize xnames
// TODO Verify xnames are unique
type CabinetLookup map[csi.CabinetKind][]string

func (cl CabinetLookup) CabinetKind(wantedCabinet string) (csi.CabinetKind, error) {
	for cabinetKind, cabinets := range cl {
		for _, cabinet := range cabinets {
			if cabinet == wantedCabinet {
				return cabinetKind, nil
			}
		}
	}

	return "", fmt.Errorf("cabinet (%s) does not exist in cabinet lookup data", wantedCabinet)
}

func (cl CabinetLookup) CabinetExists(wantedCabinet string) bool {
	for _, cabinets := range cl {
		for _, cabinet := range cabinets {
			if cabinet == wantedCabinet {
				return true
			}
		}
	}

	return false
}

func (cl CabinetLookup) CabinetClass(wantedCabinet string) (sls_common.CabinetType, error) {
	cabinetKind, err := cl.CabinetKind(wantedCabinet)
	if err != nil {
		return "", nil
	}

	return cabinetKind.Class()
}

func (cl CabinetLookup) CanCabinetContainAirCooledHardware(cabinetXname string) (bool, error) {
	cabinetKind, err := cl.CabinetKind(cabinetXname)
	if err != nil {
		return false, err
	}

	cabinetClass, err := cabinetKind.Class()
	if err != nil {
		return false, err
	}

	if cabinetClass == sls_common.ClassRiver {
		// River Cabinets can of course hold air-cooled hardware
		return true, nil
	} else if cabinetClass == sls_common.ClassHill {
		// if cabinetKind == csi.CabinetKindEX2500 {
		// 	if len(cabinetTemplate.AirCooledChassisList) >= 1 {
		// 		// This is an EX2500 cabinet with a air cooled chassis in it
		// 		return true, nil
		// 	}

		// 	// This ia an EX2500 cabinet with no air-cooled chassis
		// 	return false, fmt.Errorf("hill cabinet (EX2500) %s does not contain any air-cooled chassis", cabinetXname)
		// }

		// Traditional Hill cabinet
		return false, fmt.Errorf("hill cabinet (non EX2500) %s cannot contain air-cooled hardware", cabinetXname)

	} else if cabinetClass == sls_common.ClassMountain {
		return false, fmt.Errorf("mountain cabinet %s cannot contain air-cooled hardware", cabinetXname)
	} else {
		return false, fmt.Errorf("unknown cabinet class %s", cabinetClass)
	}
}

func (cl *CabinetLookup) DetermineRiverChassis(cabinet xnames.Cabinet) (xnames.Chassis, error) {
	// Check to see if this is even a cabinet that can have river hardware
	_, err := cl.CanCabinetContainAirCooledHardware(cabinet.String())
	if err != nil {
		return xnames.Chassis{}, err
	}

	// Next, determine if this is a standard river cabinet for a EX2500 cabinet
	// class, err := cl.CabinetClass(cabinet.String())
	// if err != nil {
	// 	return xnames.Chassis{}, err
	// }

	chassisInteger := 0
	// TODO need a source of information for this
	// if class == sls_common.ClassHill {
	// 	// This is a EX2500 cabinet with a air cooled chassis
	// 	chassisInteger = hillCabinetTemplate.AirCooledChassisList[0]
	// }

	return cabinet.Chassis(chassisInteger), nil
}

// Paddle Vendor to SLS Brand
var vendorBrandMapping = map[string]string{
	"aruba": "Aruba",
	// TODO Dell
	// TODO Mellanox
}

func main() {
	if len(os.Args) != 4 {
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

	supportedArchitectures := map[string]bool{
		"network_v2_tds": true,
		"network_v2":     true,
	}
	if !supportedArchitectures[paddle.Architecture] {
		err := fmt.Errorf("unsupported paddle architecture (%v)", paddle.Architecture)
		panic(err)
	}

	// Read in cabinet lookup
	cabinetLookupRaw, err := ioutil.ReadFile(os.Args[2])
	if err != nil {
		panic(err)
	}

	var cabinetLookup CabinetLookup
	if err := yaml.Unmarshal(cabinetLookupRaw, &cabinetLookup); err != nil {
		panic(err)
	}

	// TODO remove for testing...
	{
		jsonRaw, err := json.MarshalIndent(cabinetLookup, "", "  ")
		if err != nil {
			panic(err)
		}

		fmt.Println("DEBUG: Cabinet lookup")
		fmt.Println(string(jsonRaw))
	}

	// Read in application_node_config.yaml
	// TODO the prefixes list is not being used, as we are assuming all unknown nodes are application
	applicationNodeRaw, err := ioutil.ReadFile(os.Args[3])
	if err != nil {
		panic(err)
	}

	var applicationNodeConfig csi.SLSGeneratorApplicationNodeConfig
	if err := yaml.Unmarshal(applicationNodeRaw, &applicationNodeConfig); err != nil {
		panic(err)
	}
	if err := applicationNodeConfig.Normalize(); err != nil {
		panic(err)
	}
	if err := applicationNodeConfig.Validate(); err != nil {
		panic(err)
	}

	// Iterate over the paddle file to build of SLS data
	allHardware := map[string]sls_common.GenericHardware{}
	for _, topologyNode := range paddle.Topology {
		fmt.Println(topologyNode.Architecture, topologyNode.CommonName)

		//
		// Build the SLS hardware representation
		//
		hardware, err := buildSLSHardware(topologyNode, paddle, cabinetLookup, applicationNodeConfig)
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

		mgmtSwtichConnector, err := buildSLSMgmtSwitchConnector(hardware, topologyNode, paddle)
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
