// MIT License
//
// (C) Copyright 2022 Hewlett Packard Enterprise Development LP
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

package sls

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/Cray-HPE/cray-site-init/pkg/csi"
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
// Note when comparing hardware network information like IP address and subnets are not considered.
// TODO make striping of networking information toggable
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
		// We are ignoring fields like IPv4 fields and Model during the comparison
		// as that is something that we don't know when generating from the CCJ or CSI

		extraPropertiesA, err := DecodeHardwareExtraProperties(hardwareA)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode extra properties on (%s): %w", hardwareA.Xname, err)
		}
		extraPropertiesA = stripIpInformationFromHardware(extraPropertiesA)

		extraPropertiesB, err := DecodeHardwareExtraProperties(hardwareB)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode extra properties on (%s): %w", hardwareB.Xname, err)
		}
		extraPropertiesB = stripIpInformationFromHardware(extraPropertiesB)

		if !reflect.DeepEqual(extraPropertiesA, extraPropertiesB) {
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

func stripIpInformationFromHardware(extraPropertiesRaw interface{}) interface{} {
	switch ep := extraPropertiesRaw.(type) {
	case sls_common.ComptypeCabinet:
		ep.Networks = nil
		if cabinetKind := csi.CabinetKind(ep.Model); cabinetKind.IsModel() {
			ep.Model = ""
		}
		return ep
	case sls_common.ComptypeMgmtHLSwitch:
		ep.IP4Addr = ""
		ep.IP6Addr = ""
		ep.Model = "" // Not guaranteed that the system was installed with information about the switch model.
		return ep
	case sls_common.ComptypeMgmtSwitch:
		ep.IP4Addr = ""
		ep.IP6Addr = ""
		ep.Model = "" // Not guaranteed that the system was installed with information about the switch model.
		return ep
	}

	return extraPropertiesRaw
}
