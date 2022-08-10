package configs

import (
	"fmt"

	"github.com/Cray-HPE/cray-site-init/pkg/csi"
	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/Cray-HPE/hms-xname/xnames"
)

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
		return "", err
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
		// TODO Currently don't support adding EX2500 cabinets of any kind
		//
		// if cabinetKind == csi.CabinetKindEX2500 {
		// 	if len(cabinetTemplate.AirCooledChassisList) >= 1 {
		// 		// This is an EX2500 cabinet with a air cooled chassis in it
		// 		return true, nil
		// 	}
		//
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
	// TODO Currently don't support adding EX2500 cabinets of any kind
	// TODO need a source of information for this
	// if class == sls_common.ClassHill {
	// 	// This is a EX2500 cabinet with a air cooled chassis
	// 	chassisInteger = hillCabinetTemplate.AirCooledChassisList[0]
	// }

	return cabinet.Chassis(chassisInteger), nil
}
