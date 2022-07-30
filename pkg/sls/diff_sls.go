package sls

import (
	"fmt"
	"reflect"
	"sort"

	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
)

// Hardware present in A that is missing from B
// Set subtract operation
func HardwareSubtract(a, b sls_common.SLSState) ([]sls_common.GenericHardware, error) {
	var missingHardware []sls_common.GenericHardware

	// Build up a lookup map for the hardware in set B
	bHardwareMap := map[string]sls_common.GenericHardware{}
	for _, hardware := range b.Hardware {
		// Verify new hardware
		if _, present := bHardwareMap[hardware.Xname]; present {
			return nil, fmt.Errorf("found duplicate xname %v in set B", hardware.Xname)
		}

		bHardwareMap[hardware.Xname] = hardware
	}

	// Iterate over Set A to identify hardware not present in set B
	for _, hardware := range a.Hardware {
		if _, present := bHardwareMap[hardware.Xname]; present {
			continue
		}

		missingHardware = append(missingHardware, hardware)
	}

	// Sort the slice to make it look nice, and have a deterministic order
	sort.Slice(missingHardware, func(i, j int) bool {
		return missingHardware[i].Xname < missingHardware[j].Xname
	})

	return missingHardware, nil
}

type GenericHardwarePair struct {
	Xname     string
	HardwareA sls_common.GenericHardware
	HardwareB sls_common.GenericHardware
}

// Identify hardware
func HardwareUnion(a, b sls_common.SLSState) (identicalHardware []sls_common.GenericHardware, differingContents []GenericHardwarePair, err error) {
	// Build up a lookup map for the hardware in set B
	bHardwareMap := map[string]sls_common.GenericHardware{}
	for _, hardware := range b.Hardware {
		// Verify new hardware
		if _, present := bHardwareMap[hardware.Xname]; present {
			return nil, nil, fmt.Errorf("found duplicate xname %v in set B", hardware.Xname)
		}

		bHardwareMap[hardware.Xname] = hardware
	}

	// Iterate over Set A to identify hardware present in set B
	for _, hardwareA := range a.Hardware {
		hardwareB, present := bHardwareMap[hardwareA.Xname]
		if !present {
			continue
		}

		hardwarePair := GenericHardwarePair{
			Xname:     hardwareA.Xname,
			HardwareA: hardwareA,
			HardwareB: hardwareB,
		}

		// See if the hardware class between the 2 hardware objects is different
		if hardwareA.Class != hardwareB.Class {
			differingContents = append(differingContents, hardwarePair)

			// Don't bother checking differences in extra properties as there are already differences.
			continue
		}

		// Next check to see if the extra properties between the two hardware objects
		// TODO maybe we should ignore fields like IPv4 fields during the comparison
		// as that is something that we don't know when generating from the CCJ
		if !reflect.DeepEqual(hardwareA.ExtraPropertiesRaw, hardwareB.ExtraPropertiesRaw) {
			differingContents = append(differingContents, hardwarePair)
			continue
		}

		// If we made it here, then these 2 hardware objects must be identical
		identicalHardware = append(identicalHardware, hardwareA)
	}

	// Sort the slices to make it look nice, and have a deterministic order
	sort.Slice(identicalHardware, func(i, j int) bool {
		return identicalHardware[i].Xname < identicalHardware[j].Xname
	})
	sort.Slice(differingContents, func(i, j int) bool {
		return differingContents[i].Xname < differingContents[j].Xname
	})

	return
}
