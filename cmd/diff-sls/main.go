package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.hpe.com/sjostrand/topology-tool/pkg/sls"
)

func main() {
	if len(os.Args) != 3 {
		panic("Incorrect number of CLI args provided")
	}

	//
	// Read in SLS state files for comparison
	//

	slsStateARaw, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}

	var slsStateA sls_common.SLSState
	if err := json.Unmarshal(slsStateARaw, &slsStateA); err != nil {
		panic(err)
	}

	slsStateBRaw, err := ioutil.ReadFile(os.Args[2])
	if err != nil {
		panic(err)
	}

	var slsStateB sls_common.SLSState
	if err := json.Unmarshal(slsStateBRaw, &slsStateB); err != nil {
		panic(err)
	}

	// Identify missing hardware from either side
	hardwareMissingFromA, err := sls.HardwareSubtract(slsStateB, slsStateA)
	if err != nil {
		panic(err)
	}

	hardwareMissingFromB, err := sls.HardwareSubtract(slsStateA, slsStateB)
	if err != nil {
		panic(err)
	}

	// Identify hardware present in both states
	// Does not take into account differences in Class/ExtraProperties, just by the primary key of xname
	identicalHardware, differentContents, err := sls.HardwareUnion(slsStateA, slsStateB)
	if err != nil {
		panic(err)
	}

	//
	// Generate a report
	//

	fmt.Println("Identical hardware between A and B")
	if len(identicalHardware) == 0 {
		fmt.Println("  None")
	}
	for _, pair := range identicalHardware {
		fmt.Printf("  %s\n", pair.Xname)
	}

	fmt.Println("Common hardware between A and B with different class or extra properties")
	if len(differentContents) == 0 {
		fmt.Println("  None")
	}
	for _, pair := range differentContents {
		fmt.Printf("  %s\n", pair.Xname)
	}

	fmt.Println("Hardware missing from A (added to system)")
	if len(hardwareMissingFromA) == 0 {
		fmt.Println("  None")
	}
	for _, hardware := range hardwareMissingFromA {
		// hardwareRaw, err := json.Marshal(hardware)
		// if err != nil {
		// 	panic(err)
		// }

		fmt.Printf("  %s\n", hardware.Xname)
	}

	fmt.Println()
	fmt.Println("Hardware missing from B (removed from system)")
	if len(hardwareMissingFromB) == 0 {
		fmt.Println("  None")
	}
	for _, hardware := range hardwareMissingFromB {
		fmt.Printf("  %s\n", hardware.Xname)
	}
}
