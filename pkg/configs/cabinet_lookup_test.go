package configs

import (
	"fmt"
	"testing"

	"github.com/Cray-HPE/cray-site-init/pkg/csi"
	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/Cray-HPE/hms-xname/xnames"
	"github.com/stretchr/testify/suite"
)

type CabinetLookupTestSuite struct {
	suite.Suite
}

func (suite *CabinetLookupTestSuite) TestCabinetExists() {
	cabinetLookup := CabinetLookup{
		csi.CabinetKindRiver:    []string{"x3000", "x3001"},
		csi.CabinetKindHill:     []string{"x9000"},
		csi.CabinetKindMountain: []string{"x1000"},
	}

	for _, cabinet := range []string{"x1000", "x3000", "x3001", "x9000"} {
		suite.True(cabinetLookup.CabinetExists(cabinet), "Cabinet %s does not exist", cabinet)
	}
}

func (suite *CabinetLookupTestSuite) TestCabinetExists_NotFound() {
	cabinetLookup := CabinetLookup{
		csi.CabinetKindRiver:    []string{"x3000", "x3001"},
		csi.CabinetKindHill:     []string{"x9000"},
		csi.CabinetKindMountain: []string{"x1000"},
	}

	for _, cabinet := range []string{"x1001", "x3002", "x9001"} {
		suite.False(cabinetLookup.CabinetExists(cabinet), "Cabinet %s should not exist", cabinet)
	}

}

func (suite *CabinetLookupTestSuite) TestCabinetClass() {
	cabinetLookup := CabinetLookup{
		csi.CabinetKindRiver:    []string{"x3000", "x3001"},
		csi.CabinetKindHill:     []string{"x9000"},
		csi.CabinetKindMountain: []string{"x1000"},
	}

	// River
	cabinetClass, err := cabinetLookup.CabinetClass("x3000")
	suite.NoError(err)
	suite.Equal(sls_common.ClassRiver, cabinetClass, "x3000 has unexpected cabinet class")

	// Hill
	cabinetClass, err = cabinetLookup.CabinetClass("x9000")
	suite.NoError(err)
	suite.Equal(sls_common.ClassHill, cabinetClass, "x9000 has unexpected cabinet class")

	// Mountain
	cabinetClass, err = cabinetLookup.CabinetClass("x1000")
	suite.NoError(err)
	suite.Equal(sls_common.ClassMountain, cabinetClass, "x1000 has unexpected cabinet class")
}

func (suite *CabinetLookupTestSuite) TestCabinetClass_NotFound() {
	cabinetLookup := CabinetLookup{
		csi.CabinetKindRiver:    []string{"x3000", "x3001"},
		csi.CabinetKindHill:     []string{"x9000"},
		csi.CabinetKindMountain: []string{"x1000"},
	}

	for _, cabinet := range []string{"x1001", "x3002", "x9001"} {
		_, err := cabinetLookup.CabinetClass(cabinet)
		suite.EqualError(err, fmt.Sprintf("cabinet (%s) does not exist in cabinet lookup data", cabinet))
	}
}

func (suite *CabinetLookupTestSuite) TestCanCabinetContainAirCooledHardware_RiverCabinet() {
	cabinetLookup := CabinetLookup{
		csi.CabinetKindRiver:    []string{"x3000", "x3001"},
		csi.CabinetKindHill:     []string{"x9000"},
		csi.CabinetKindMountain: []string{"x1000"},
	}

	ok, err := cabinetLookup.CanCabinetContainAirCooledHardware("x3000")
	suite.NoError(err)
	suite.True(ok)
}

func (suite *CabinetLookupTestSuite) TestCanCabinetContainAirCooledHardware_MountainCabinet() {
	cabinetLookup := CabinetLookup{
		csi.CabinetKindRiver:    []string{"x3000", "x3001"},
		csi.CabinetKindHill:     []string{"x9000"},
		csi.CabinetKindMountain: []string{"x1000"},
	}

	ok, err := cabinetLookup.CanCabinetContainAirCooledHardware("x1000")
	suite.EqualError(err, "mountain cabinet x1000 cannot contain air-cooled hardware")
	suite.False(ok)
}

func (suite *CabinetLookupTestSuite) TestCanCabinetContainAirCooledHardware_HillCabinet() {
	cabinetLookup := CabinetLookup{
		csi.CabinetKindRiver:    []string{"x3000", "x3001"},
		csi.CabinetKindHill:     []string{"x9000"},
		csi.CabinetKindMountain: []string{"x1000"},
	}

	ok, err := cabinetLookup.CanCabinetContainAirCooledHardware("x9000")
	suite.EqualError(err, "hill cabinet (non EX2500) x9000 cannot contain air-cooled hardware")
	suite.False(ok)
}

// TODO Currently don't support adding EX2500 cabinets of any kind
//
// func (suite *CabinetLookupTestSuite) TestCanCabinetContainAirCooledHardware_EX2500_NoAirCooledChassis() {
// 	cabinetLookup := CabinetLookup{
// 		csi.CabinetKindRiver:    []string{"x3000", "x3001"},
// 		csi.CabinetKindHill:     []string{"x9000"},
// 		csi.CabinetKindMountain: []string{"x1000"},
// 		csi.CabinetKindEX2500:   []string{"x8000"},
// 	}
//
// 	ok, err := cabinetLookup.CanCabinetContainAirCooledHardware("x8000")
// 	suite.EqualError(err, "hill cabinet (EX2500) x8000 does not contain any air-cooled chassis")
// 	suite.False(ok)
// }
//
// func (suite *CabinetLookupTestSuite) TestCanCabinetContainAirCooledHardware_EX2500_AirCooledChassis() {
// 	cabinetLookup := CabinetLookup{
// 		csi.CabinetKindRiver:    []string{"x3000", "x3001"},
// 		csi.CabinetKindHill:     []string{"x9000"},
// 		csi.CabinetKindMountain: []string{"x1000"},
// 		csi.CabinetKindEX2500:   []string{"x8000"},
// 	}
//
// 	ok, err := cabinetLookup.CanCabinetContainAirCooledHardware("x8000")
// 	suite.NoError(err)
// 	suite.True(ok)
// }

func (suite *CabinetLookupTestSuite) TestCanCabinetContainAirCooledHardware_UnknownCabinet() {
	cabinetLookup := CabinetLookup{
		csi.CabinetKindRiver:    []string{"x3000", "x3001"},
		csi.CabinetKindHill:     []string{"x9000"},
		csi.CabinetKindMountain: []string{"x1000"},
	}

	ok, err := cabinetLookup.CanCabinetContainAirCooledHardware("x1234")
	suite.Error(err)
	suite.False(ok)
}

func (suite *CabinetLookupTestSuite) TestDetermineRiverChassis_RiverCabinet() {
	cabinetLookup := CabinetLookup{
		csi.CabinetKindRiver:    []string{"x3000", "x3001"},
		csi.CabinetKindHill:     []string{"x9000"},
		csi.CabinetKindMountain: []string{"x1000"},
	}

	chassis, err := cabinetLookup.DetermineRiverChassis(xnames.Cabinet{Cabinet: 3000})
	suite.NoError(err)
	suite.Equal(xnames.FromString("x3000c0"), chassis)
}

func (suite *CabinetLookupTestSuite) TestDetermineRiverChassis_HillCabinet() {
	cabinetLookup := CabinetLookup{
		csi.CabinetKindRiver:    []string{"x3000", "x3001"},
		csi.CabinetKindHill:     []string{"x9000"},
		csi.CabinetKindMountain: []string{"x1000"},
	}

	_, err := cabinetLookup.DetermineRiverChassis(xnames.Cabinet{Cabinet: 9000})
	suite.EqualError(err, "hill cabinet (non EX2500) x9000 cannot contain air-cooled hardware")
}

// TODO Currently don't support adding EX2500 cabinets of any kind
//
// func (suite *CabinetLookupTestSuite) TestDetermineRiverChassis_EX2500Cabinet() {
// 	cabinetLookup := CabinetLookup{
// 		csi.CabinetKindRiver:    []string{"x3000", "x3001"},
// 		csi.CabinetKindHill:     []string{"x9000"},
// 		csi.CabinetKindMountain: []string{"x1000"},
// 	}
//
// 	chassis, err := cabinetLookup.DetermineRiverChassis(xnames.Cabinet{Cabinet: 5004})
// 	suite.NoError(err)
// 	suite.Equal(xnames.FromString("x5004c4"), chassis)
// }

func (suite *CabinetLookupTestSuite) TestDetermineRiverChassis_MountainCabinet() {
	cabinetLookup := CabinetLookup{
		csi.CabinetKindRiver:    []string{"x3000", "x3001"},
		csi.CabinetKindHill:     []string{"x9000"},
		csi.CabinetKindMountain: []string{"x1000"},
	}

	_, err := cabinetLookup.DetermineRiverChassis(xnames.Cabinet{Cabinet: 1000})
	suite.EqualError(err, "mountain cabinet x1000 cannot contain air-cooled hardware")
}

func (suite *CabinetLookupTestSuite) TestDetermineRiverChassis_InvalidCabinet() {
	cabinetLookup := CabinetLookup{
		csi.CabinetKindRiver:    []string{"x3000", "x3001"},
		csi.CabinetKindHill:     []string{"x9000"},
		csi.CabinetKindMountain: []string{"x1000"},
	}

	_, err := cabinetLookup.DetermineRiverChassis(xnames.Cabinet{Cabinet: 1234})
	suite.Error(err)
}

func TestCabinetLookupTestSuite(t *testing.T) {
	suite.Run(t, new(CabinetLookupTestSuite))
}
