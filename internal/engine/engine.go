package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Cray-HPE/cray-site-init/pkg/csi"
	sls_client "github.com/Cray-HPE/hms-sls/pkg/sls-client"
	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/Cray-HPE/hms-xname/xnametypes"
	"github.hpe.com/sjostrand/topology-tool/pkg/ccj"
	"github.hpe.com/sjostrand/topology-tool/pkg/configs"
	"github.hpe.com/sjostrand/topology-tool/pkg/sls"
)

type TopologyEngine struct {
	Input     EngineInput
	SLSClient *sls_client.SLSClient
}

type EngineInput struct {
	Paddle                ccj.Paddle
	CabinetLookup         configs.CabinetLookup
	ApplicationNodeConfig csi.SLSGeneratorApplicationNodeConfig

	CurrentSLSState sls_common.SLSState
}

func (te *TopologyEngine) Run(ctx context.Context) error {
	// Build up the expected SLS hardware state from the provided CCJ
	expectedSLSState, err := te.buildExpectedHardwareState()
	if err != nil {
		return fmt.Errorf("failed to build expected SLS hardware state: %w", err)
	}

	//
	// Compare the current hardware state with the expected hardware state
	//

	// Identify missing hardware from either side
	hardwareRemoved, err := sls.HardwareSubtract(te.Input.CurrentSLSState, expectedSLSState)
	if err != nil {
		panic(err)
	}

	hardwareAdded, err := sls.HardwareSubtract(expectedSLSState, te.Input.CurrentSLSState)
	if err != nil {
		panic(err)
	}

	// Identify hardware present in both states
	// Does not take into account differences in Class/ExtraProperties, just by the primary key of xname
	identicalHardware, hardwareWithDifferingValues, err := sls.HardwareUnion(te.Input.CurrentSLSState, expectedSLSState)
	if err != nil {
		panic(err)
	}

	te.displayHardwareComparisonReport(hardwareRemoved, hardwareAdded, identicalHardware, hardwareWithDifferingValues)

	//
	// GUARD RAILS - If hardware is removed of has differing values then
	// DO NOT PROCEED.
	//
	// This is put in place as the first use for this tool is to add river cabinets to the system.
	//
	if len(hardwareRemoved) != 0 {
		return fmt.Errorf("refusing to continue, found hardware was removed from the system. Please reconcile the current system state with the systems CCJ/SHCD")
	}

	if len(hardwareWithDifferingValues) != 0 {
		return fmt.Errorf("refusing to continue, found hardware with differing values (Class and/or ExtraProperties). Please reconcile the differences")
	}

	// TODO Disallow for mountain cabinet additions? Or for right now just ignore them all add stuff with the River class

	//
	// Check for new hardware additions that require changes to the network
	//

	// First look for any new cabinets, and allocation an subnet for them
	for _, hardware := range hardwareAdded {
		if hardware.TypeString == xnametypes.Cabinet {
			// Allocation Cabinet Subnet
		}
	}

	// Allocation Switch IPs
	for _, hardware := range hardwareAdded {
		hmsType := hardware.TypeString
		if hmsType == xnametypes.CDUMgmtSwitch || hmsType == xnametypes.MgmtHLSwitch || hmsType == xnametypes.MgmtSwitch {
			// Allocation IP for Switch
		}
	}

	return nil
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

func (te *TopologyEngine) displayHardwareComparisonReport(hardwareRemoved, hardwareAdded []sls_common.GenericHardware, identicalHardware, hardwareWithDifferingValues []sls.GenericHardwarePair) {
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
