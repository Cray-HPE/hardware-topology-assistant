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

package ccj

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/Cray-HPE/cray-site-init/pkg/csi"
	"github.com/Cray-HPE/hms-xname/xnames"
	"github.hpe.com/sjostrand/topology-tool/pkg/configs"
)

func DetermineCabinetLookup(paddle Paddle) (configs.CabinetLookup, error) {
	cabinetLookup := configs.CabinetLookup{}

	//
	// Determine what liquid-cooled cabinets have
	//
	liquidCooledCabinets := map[string]bool{}
	cabinetChassisList := map[string][]int{}

	// Find all liquid cooled cabinets and what chassis they have
	for _, topologyNode := range paddle.Topology {
		if topologyNode.Architecture != "cmm" {
			continue
		}

		// Determine the xname of the cabinet
		cabinetOrdinal, err := extractNumber(topologyNode.Location.Rack)
		if err != nil {
			return configs.CabinetLookup{}, fmt.Errorf("unable to extract cabinet ordinal due to: %w", err)
		}
		cabinet := xnames.Cabinet{
			Cabinet: cabinetOrdinal,
		}

		chassisOrdinal, err := extractNumber(topologyNode.Location.Elevation)
		if err != nil {
			return configs.CabinetLookup{}, fmt.Errorf("unable to extract chassis ordinal due to: %w", err)
		}

		liquidCooledCabinets[cabinet.String()] = true
		cabinetChassisList[cabinet.String()] = append(cabinetChassisList[cabinet.String()], chassisOrdinal)
	}

	// Apply some heuristics to infer the type of cabinet this is
	for cabinet := range liquidCooledCabinets {
		chassisList := cabinetChassisList[cabinet]
		sort.Ints(chassisList)

		var kind csi.CabinetKind
		if reflect.DeepEqual(chassisList, []int{0, 1, 2, 3, 4, 5, 6, 7}) {
			kind = csi.CabinetKindMountain
		} else if reflect.DeepEqual(chassisList, []int{1, 3}) {
			kind = csi.CabinetKindHill
		} else if reflect.DeepEqual(chassisList, []int{0}) {
			kind = csi.CabinetKindEX2500
		} else if reflect.DeepEqual(chassisList, []int{0, 1}) {
			kind = csi.CabinetKindEX2500
		} else if reflect.DeepEqual(chassisList, []int{0, 1, 3}) {
			kind = csi.CabinetKindEX2500
		} else {
			return configs.CabinetLookup{}, fmt.Errorf("unable to infer liquid-cooled cabinet kind with chassis list (%v)", chassisList)
		}

		cabinetLookup[kind] = append(cabinetLookup[kind], cabinet)

	}

	//
	// Determine River cabinets
	//
	riverCabinets := map[string]bool{}
	for _, topologyNode := range paddle.Topology {
		// If this is located with in a CDU skip it
		if strings.HasPrefix(strings.ToLower(topologyNode.Location.Rack), "cdu") {
			continue
		}

		// Determine the xname of the cabinet
		cabinetOrdinal, err := extractNumber(topologyNode.Location.Rack)
		if err != nil {
			return configs.CabinetLookup{}, fmt.Errorf("unable to extract cabinet ordinal due to: %w", err)
		}
		cabinet := xnames.Cabinet{
			Cabinet: cabinetOrdinal,
		}

		if liquidCooledCabinets[cabinet.String()] {
			continue
		}

		// This must be a river cabinet then
		riverCabinets[cabinet.String()] = true
	}

	for cabinet := range riverCabinets {
		cabinetLookup[csi.CabinetKindRiver] = append(cabinetLookup[csi.CabinetKindRiver], cabinet)

	}

	return cabinetLookup, nil
}
