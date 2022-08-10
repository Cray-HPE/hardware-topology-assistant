package ccj

import (
	"testing"

	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/Cray-HPE/hms-xname/xnames"
	"github.com/stretchr/testify/suite"
	"github.hpe.com/sjostrand/topology-tool/pkg/configs"
)

type SLSStateGeneratorTestSuite struct {
	suite.Suite
}

// TODO
// func (suite *SLSStateGeneratorTestSuite) TestBuildExpectedHardwareState() {
//
// }

func (suite *SLSStateGeneratorTestSuite) TestPDUController_p0() {
	location := Location{Rack: "x3000", Elevation: "p0"}
	hardware, err := buildSLSPDUController(location)
	suite.NoError(err)

	expectedHardware := sls_common.NewGenericHardware("x3000m0", sls_common.ClassRiver, nil)
	suite.Equal(expectedHardware, hardware)
}

func (suite *SLSStateGeneratorTestSuite) TestPDUController_p1() {
	location := Location{Rack: "x3000", Elevation: "p1"}
	hardware, err := buildSLSPDUController(location)
	suite.NoError(err)

	expectedHardware := sls_common.NewGenericHardware("x3000m1", sls_common.ClassRiver, nil)
	suite.Equal(expectedHardware, hardware)
}

func (suite *SLSStateGeneratorTestSuite) TestPDUController_InvalidRack() {
	location := Location{Rack: "x", Elevation: "p1"}
	_, err := buildSLSPDUController(location)
	suite.EqualError(err, "unable to extract cabinet ordinal due to: unexpected number of matches 0 expected 2")
}

func (suite *SLSStateGeneratorTestSuite) TestPDUController_InvalidElevation() {
	location := Location{Rack: "x3000", Elevation: "p"}
	_, err := buildSLSPDUController(location)
	suite.EqualError(err, "unable to extract pdu ordinal due to: unexpected number of matches 0 expected 2")
}

func (suite *SLSStateGeneratorTestSuite) TestSlingshotHSNSwitch() {
	location := Location{Rack: "x3000", Elevation: "u39"}
	hardware, err := buildSLSSlingshotHSNSwitch(location)
	suite.NoError(err)

	expectedHardware := sls_common.NewGenericHardware("x3000c0r39b0", sls_common.ClassRiver, sls_common.ComptypeRtrBmc{
		Username: "vault://hms-creds/x3000c0r39b0",
		Password: "vault://hms-creds/x3000c0r39b0",
	})
	suite.Equal(expectedHardware, hardware)
}

func (suite *SLSStateGeneratorTestSuite) TestCMC() {
	location := Location{Rack: "x3000", Elevation: "u01"}
	hardware, err := buildSLSCMC(location)
	suite.NoError(err)

	expectedHardware := sls_common.NewGenericHardware("x3000c0s1b999", sls_common.ClassRiver, nil)
	suite.Equal(expectedHardware, hardware)
}

func (suite *SLSStateGeneratorTestSuite) TestNode_Compute() {
	topologyNode := TopologyNode{
		CommonName:   "cn003",
		ID:           47,
		Architecture: "river_compute_node",
		Model:        "river_compute_node",
		Type:         "node",
		Vendor:       "none",
		Ports: []Port{
			{
				Port:       1,
				Speed:      1,
				Slot:       "cmc",
				DestNodeID: 46,
				DestPort:   2,
				DestSlot:   "cmc",
			},
		},
		Location: Location{
			Rack:      "x3000",
			Elevation: "u26",
			Parent:    "SubRack-001-CMC",
		},
	}

	topologyNodeCMC := TopologyNode{
		CommonName:   "SubRack001-CMC",
		ID:           46,
		Architecture: "subrack",
		Model:        "subrack",
		Type:         "subrack",
		Vendor:       "none",
		Location: Location{
			Rack:      "x3000",
			Elevation: "u25",
		},
	}

	paddle := Paddle{
		Topology: []TopologyNode{topologyNode, topologyNodeCMC},
	}

	hardware, err := buildSLSNode(topologyNode, paddle, nil)
	suite.NoError(err)

	expectedHardware := sls_common.NewGenericHardware("x3000c0s25b3n0", sls_common.ClassRiver, sls_common.ComptypeNode{
		Role:    "Compute",
		NID:     3,
		Aliases: []string{"nid000003"},
	})
	suite.Equal(expectedHardware, hardware)
}

func (suite *SLSStateGeneratorTestSuite) TestNode_Application() {
	topologyNode := TopologyNode{
		CommonName:   "uan001",
		ID:           21,
		Architecture: "river_ncn_node_4_port",
		Model:        "river_ncn_node_4_port",
		Type:         "server",
		Vendor:       "hpe",
		Location: Location{
			Rack:      "x3000",
			Elevation: "u15",
		},
	}

	applicationNodeMetadata := configs.ApplicationNodeMetadataMap{
		"x3000c0s15b0n0": {
			SubRole: "UAN",
			Aliases: []string{"uan01"},
		},
	}

	paddle := Paddle{
		Topology: []TopologyNode{topologyNode, topologyNode},
	}

	hardware, err := buildSLSNode(topologyNode, paddle, applicationNodeMetadata)
	suite.NoError(err)

	expectedHardware := sls_common.NewGenericHardware("x3000c0s15b0n0", sls_common.ClassRiver, sls_common.ComptypeNode{
		Role:    "Application",
		SubRole: "UAN",
		Aliases: []string{"uan01"},
	})
	suite.Equal(expectedHardware, hardware)
}

func (suite *SLSStateGeneratorTestSuite) TestMgmtSwitch_Aruba() {
	topologyNode := TopologyNode{
		CommonName:   "sw-leaf-bmc-004",
		ID:           37,
		Architecture: "river_bmc_leaf",
		Model:        "6300M_JL762A",
		Type:         "switch",
		Vendor:       "aruba",
		Location: Location{
			Rack:      "x3001",
			Elevation: "u32",
		},
	}

	hardware, err := buildSLSMgmtSwitch(topologyNode, nil)
	suite.NoError(err)

	expectedHardware := sls_common.NewGenericHardware("x3001c0w32", sls_common.ClassRiver, sls_common.ComptypeMgmtSwitch{
		Brand:            "Aruba",
		Model:            "6300M_JL762A",
		Aliases:          []string{"sw-leaf-bmc-004"},
		SNMPAuthPassword: "vault://hms-creds/x3001c0w32",
		SNMPAuthProtocol: "MD5",
		SNMPPrivPassword: "vault://hms-creds/x3001c0w32",
		SNMPPrivProtocol: "DES",
		SNMPUsername:     "testuser",
	})
	suite.Equal(expectedHardware, hardware)
}

func (suite *SLSStateGeneratorTestSuite) TestMgmtSwitch_Dell() {
	topologyNode := TopologyNode{
		CommonName:   "sw-leaf-bmc-004",
		ID:           37,
		Architecture: "river_bmc_leaf",
		Model:        "S3048-ON",
		Type:         "switch",
		Vendor:       "dell",
		Location: Location{
			Rack:      "x3001",
			Elevation: "u32",
		},
	}

	hardware, err := buildSLSMgmtSwitch(topologyNode, nil)
	suite.NoError(err)

	expectedHardware := sls_common.NewGenericHardware("x3001c0w32", sls_common.ClassRiver, sls_common.ComptypeMgmtSwitch{
		Brand:            "Dell",
		Model:            "S3048-ON",
		Aliases:          []string{"sw-leaf-bmc-004"},
		SNMPAuthPassword: "vault://hms-creds/x3001c0w32",
		SNMPAuthProtocol: "MD5",
		SNMPPrivPassword: "vault://hms-creds/x3001c0w32",
		SNMPPrivProtocol: "DES",
		SNMPUsername:     "testuser",
	})
	suite.Equal(expectedHardware, hardware)
}

func (suite *SLSStateGeneratorTestSuite) TestMgmtHLSwitch_Aruba() {
	topologyNode := TopologyNode{
		CommonName:   "sw-spine-002",
		ID:           12,
		Architecture: "spine",
		Model:        "8325_JL627A",
		Type:         "switch",
		Vendor:       "aruba",
		Location: Location{
			Rack:      "x3000",
			Elevation: "u38",
		},
	}

	hardware, err := buildSLSMgmtHLSwitch(topologyNode, nil)
	suite.NoError(err)

	expectedHardware := sls_common.NewGenericHardware("x3000c0h38s1", sls_common.ClassRiver, sls_common.ComptypeMgmtHLSwitch{
		Brand:   "Aruba",
		Model:   "8325_JL627A",
		Aliases: []string{"sw-spine-002"},
	})
	suite.Equal(expectedHardware, hardware)
}

func (suite *SLSStateGeneratorTestSuite) TestMgmtHLSwitch_Mellanox_Left() {
	topologyNode := TopologyNode{
		CommonName:   "sw-spine-002",
		ID:           12,
		Architecture: "spine",
		Model:        "SN2700",
		Type:         "switch",
		Vendor:       "mellanox",
		Location: Location{
			Rack:        "x3000",
			Elevation:   "u38",
			SubLocation: "L",
		},
	}

	hardware, err := buildSLSMgmtHLSwitch(topologyNode, nil)
	suite.NoError(err)

	expectedHardware := sls_common.NewGenericHardware("x3000c0h38s1", sls_common.ClassRiver, sls_common.ComptypeMgmtHLSwitch{
		Brand:   "Mellanox",
		Model:   "SN2700",
		Aliases: []string{"sw-spine-002"},
	})
	suite.Equal(expectedHardware, hardware)
}

func (suite *SLSStateGeneratorTestSuite) TestMgmtHLSwitch_Mellanox_Right() {
	topologyNode := TopologyNode{
		CommonName:   "sw-spine-002",
		ID:           12,
		Architecture: "spine",
		Model:        "SN2700",
		Type:         "switch",
		Vendor:       "mellanox",
		Location: Location{
			Rack:        "x3000",
			Elevation:   "u38",
			SubLocation: "R",
		},
	}

	hardware, err := buildSLSMgmtHLSwitch(topologyNode, nil)
	suite.NoError(err)

	expectedHardware := sls_common.NewGenericHardware("x3000c0h38s2", sls_common.ClassRiver, sls_common.ComptypeMgmtHLSwitch{
		Brand:   "Mellanox",
		Model:   "SN2700",
		Aliases: []string{"sw-spine-002"},
	})
	suite.Equal(expectedHardware, hardware)
}

func (suite *SLSStateGeneratorTestSuite) TestMgmtHLSwitch_Arista() {
	topologyNode := TopologyNode{
		CommonName:   "sw-edge-001",
		ID:           32,
		Architecture: "customer_edge_router",
		Model:        "customer_edge_router",
		Type:         "switch",
		Vendor:       "none",
		Location: Location{
			Rack:      "x3000",
			Elevation: "u18",
		},
	}

	hardware, err := buildSLSMgmtHLSwitch(topologyNode, nil)
	suite.NoError(err)

	expectedHardware := sls_common.NewGenericHardware("x3000c0h18s1", sls_common.ClassRiver, sls_common.ComptypeMgmtHLSwitch{
		Brand:   "Arista",
		Aliases: []string{"sw-edge-001"},
	})
	suite.Equal(expectedHardware, hardware)
}

func (suite *SLSStateGeneratorTestSuite) TestCDUMgmtSwitch_Aruba() {
	topologyNode := TopologyNode{
		CommonName:   "sw-cdu-002",
		ID:           0,
		Architecture: "mountain_compute_leaf",
		Model:        "8360_JL706A",
		Type:         "switch",
		Vendor:       "aruba",
		Location: Location{
			Rack:      "cdu0",
			Elevation: "su2",
		},
	}

	hardware, err := buildSLSCDUMgmtSwitch(topologyNode, nil)
	suite.NoError(err)

	expectedHardware := sls_common.NewGenericHardware("d0w2", sls_common.ClassMountain, sls_common.ComptypeCDUMgmtSwitch{
		Brand:   "Aruba",
		Model:   "8360_JL706A",
		Aliases: []string{"sw-cdu-002"},
	})
	suite.Equal(expectedHardware, hardware)
}

func (suite *SLSStateGeneratorTestSuite) TestCDUMgmtSwitch_Dell() {
	topologyNode := TopologyNode{
		CommonName:   "sw-cdu-002",
		ID:           0,
		Architecture: "mountain_compute_leaf",
		Model:        "S4148T-ON",
		Type:         "switch",
		Vendor:       "dell",
		Location: Location{
			Rack:      "cdu0",
			Elevation: "su2",
		},
	}

	hardware, err := buildSLSCDUMgmtSwitch(topologyNode, nil)
	suite.NoError(err)

	expectedHardware := sls_common.NewGenericHardware("d0w2", sls_common.ClassMountain, sls_common.ComptypeCDUMgmtSwitch{
		Brand:   "Dell",
		Model:   "S4148T-ON",
		Aliases: []string{"sw-cdu-002"},
	})
	suite.Equal(expectedHardware, hardware)
}

func TestSLSStateGeneratorTestSuite(t *testing.T) {
	suite.Run(t, new(SLSStateGeneratorTestSuite))
}

type BuildNodeExtraPropertiesTestSuite struct {
	suite.Suite
}

func (suite *BuildNodeExtraPropertiesTestSuite) TestMaster() {
	topologyNode := TopologyNode{
		CommonName:   "ncn-m003",
		ID:           29,
		Architecture: "river_ncn_node_4_port",
		Model:        "river_ncn_node_4_port",
		Type:         "server",
		Vendor:       "hpe",
		Location: Location{
			Rack:      "x3000",
			Elevation: "u03",
		},
	}

	extraProperties, err := BuildNodeExtraProperties(topologyNode)
	suite.NoError(err)

	expectedExtraProperties := sls_common.ComptypeNode{
		Role:    "Management",
		SubRole: "Master",
		Aliases: []string{"ncn-m003"},
	}
	suite.Equal(expectedExtraProperties, extraProperties)
}
func (suite *BuildNodeExtraPropertiesTestSuite) TestWorker() {
	topologyNode := TopologyNode{
		CommonName:   "ncn-w003",
		ID:           26,
		Architecture: "river_ncn_node_4_port",
		Model:        "river_ncn_node_4_port",
		Type:         "server",
		Vendor:       "hpe",
		Location: Location{
			Rack:      "x3000",
			Elevation: "u06",
		},
	}

	extraProperties, err := BuildNodeExtraProperties(topologyNode)
	suite.NoError(err)

	expectedExtraProperties := sls_common.ComptypeNode{
		Role:    "Management",
		SubRole: "Worker",
		Aliases: []string{"ncn-w003"},
	}
	suite.Equal(expectedExtraProperties, extraProperties)
}

func (suite *BuildNodeExtraPropertiesTestSuite) TestStorage() {
	topologyNode := TopologyNode{
		CommonName:   "ncn-s003",
		ID:           22,
		Architecture: "river_ncn_node_4_port",
		Model:        "river_ncn_node_4_port",
		Type:         "server",
		Vendor:       "hpe",
		Location: Location{
			Rack:      "x3000",
			Elevation: "u10",
		},
	}

	extraProperties, err := BuildNodeExtraProperties(topologyNode)
	suite.NoError(err)

	expectedExtraProperties := sls_common.ComptypeNode{
		Role:    "Management",
		SubRole: "Storage",
		Aliases: []string{"ncn-s003"},
	}
	suite.Equal(expectedExtraProperties, extraProperties)
}

func (suite *BuildNodeExtraPropertiesTestSuite) TestCompute() {
	topologyNode := TopologyNode{
		CommonName:   "cn003",
		ID:           47,
		Architecture: "river_compute_node",
		Model:        "river_compute_node",
		Type:         "node",
		Vendor:       "none",
		Location: Location{
			Rack:      "x3000",
			Elevation: "u10",
		},
	}

	extraProperties, err := BuildNodeExtraProperties(topologyNode)
	suite.NoError(err)

	expectedExtraProperties := sls_common.ComptypeNode{
		Role:    "Compute",
		NID:     3,
		Aliases: []string{"nid000003"},
	}
	suite.Equal(expectedExtraProperties, extraProperties)
}

func (suite *BuildNodeExtraPropertiesTestSuite) TestApplication() {
	topologyNode := TopologyNode{
		CommonName:   "uan001",
		ID:           21,
		Architecture: "river_ncn_node_4_port",
		Model:        "river_ncn_node_4_port",
		Type:         "server",
		Vendor:       "hpe",
		Location: Location{
			Rack:      "x3000",
			Elevation: "u15",
		},
	}

	extraProperties, err := BuildNodeExtraProperties(topologyNode)
	suite.NoError(err)

	expectedExtraProperties := sls_common.ComptypeNode{
		Role: "Application",
	}
	suite.Equal(expectedExtraProperties, extraProperties)
}

func (suite *BuildNodeExtraPropertiesTestSuite) TestInvalidHardware() {
	topologyNode := TopologyNode{
		CommonName:   "pdu-x3001-000",
		ID:           56,
		Architecture: "pdu",
		Model:        "pdu",
		Type:         "none",
		Vendor:       "hpe",
		Location: Location{
			Rack:      "x3001",
			Elevation: "p0",
		},
	}

	_, err := BuildNodeExtraProperties(topologyNode)
	suite.Errorf(err, "unexpected topology node type (pdu) expected (server or node)")
}

func TestBuildNodeExtraPropertiesTestSuite(t *testing.T) {
	suite.Run(t, new(BuildNodeExtraPropertiesTestSuite))
}

type BuildNodeXnameTestSuite struct {
	suite.Suite
}

func (suite *BuildNodeXnameTestSuite) TestSingleChassisNode() {
	topologyNode := TopologyNode{
		CommonName:   "ncn-s003",
		ID:           22,
		Architecture: "river_ncn_node_4_port",
		Model:        "river_ncn_node_4_port",
		Type:         "server",
		Vendor:       "hpe",
		Location: Location{
			Rack:      "x3000",
			Elevation: "u10",
		},
	}

	extraProperties := sls_common.ComptypeNode{
		Role:    "Management",
		SubRole: "Storage",
		Aliases: []string{"ncn-s003"},
	}

	paddle := Paddle{
		Topology: []TopologyNode{topologyNode},
	}

	xname, err := BuildNodeXname(topologyNode, paddle, extraProperties)
	suite.NoError(err)

	expectedXname := xnames.Node{
		Cabinet:       3000,
		Chassis:       0,
		ComputeModule: 10,
		NodeBMC:       0,
		Node:          0,
	}

	suite.Equal(expectedXname, xname)
}

func (suite *BuildNodeXnameTestSuite) TestDualChassisNode_Left() {
	// Apollo 6500 645XL
	topologyNode := TopologyNode{
		CommonName:   "uan001",
		ID:           21,
		Architecture: "river_ncn_node_4_port",
		Model:        "river_ncn_node_4_port",
		Type:         "server",
		Vendor:       "hpe",
		Location: Location{
			Rack:        "x3000",
			Elevation:   "u15",
			SubLocation: "L",
		},
	}

	extraProperties, err := BuildNodeExtraProperties(topologyNode)
	suite.NoError(err)

	paddle := Paddle{
		Topology: []TopologyNode{topologyNode},
	}

	xname, err := BuildNodeXname(topologyNode, paddle, extraProperties)
	suite.NoError(err)

	expectedXname := xnames.Node{
		Cabinet:       3000,
		Chassis:       0,
		ComputeModule: 15,
		NodeBMC:       1,
		Node:          0,
	}

	suite.Equal(expectedXname, xname)
}

func (suite *BuildNodeXnameTestSuite) TestDualChassisNode_Right() {
	// Apollo 6500 645XL
	// Apollo 6500 645XL
	topologyNode := TopologyNode{
		CommonName:   "uan001",
		ID:           21,
		Architecture: "river_ncn_node_4_port",
		Model:        "river_ncn_node_4_port",
		Type:         "server",
		Vendor:       "hpe",
		Location: Location{
			Rack:        "x3000",
			Elevation:   "u15",
			SubLocation: "R",
		},
	}

	extraProperties, err := BuildNodeExtraProperties(topologyNode)
	suite.NoError(err)

	paddle := Paddle{
		Topology: []TopologyNode{topologyNode},
	}

	xname, err := BuildNodeXname(topologyNode, paddle, extraProperties)
	suite.NoError(err)

	expectedXname := xnames.Node{
		Cabinet:       3000,
		Chassis:       0,
		ComputeModule: 15,
		NodeBMC:       2,
		Node:          0,
	}

	suite.Equal(expectedXname, xname)
}

func (suite *BuildNodeXnameTestSuite) TestQuadChassisNode() {
	// Gigabyte and Intel computes

	topologyNode := TopologyNode{
		CommonName:   "cn003",
		ID:           47,
		Architecture: "river_compute_node",
		Model:        "river_compute_node",
		Type:         "node",
		Vendor:       "none",
		Ports: []Port{
			{
				Port:       1,
				Speed:      1,
				Slot:       "cmc",
				DestNodeID: 46,
				DestPort:   2,
				DestSlot:   "cmc",
			},
		},
		Location: Location{
			Rack:      "x3000",
			Elevation: "u26",
			Parent:    "SubRack-001-CMC",
		},
	}

	topologyNodeCMC := TopologyNode{
		CommonName:   "SubRack001-CMC",
		ID:           46,
		Architecture: "subrack",
		Model:        "subrack",
		Type:         "subrack",
		Vendor:       "none",
		Location: Location{
			Rack:      "x3000",
			Elevation: "u25",
		},
	}

	extraProperties := sls_common.ComptypeNode{
		Role:    "Compute",
		NID:     3,
		Aliases: []string{"nid000003"},
	}

	paddle := Paddle{
		Topology: []TopologyNode{topologyNode, topologyNodeCMC},
	}

	xname, err := BuildNodeXname(topologyNode, paddle, extraProperties)
	suite.NoError(err)

	expectedXname := xnames.Node{
		Cabinet:       3000,
		Chassis:       0,
		ComputeModule: 25,
		NodeBMC:       3,
		Node:          0,
	}

	suite.Equal(expectedXname, xname)
}

func (suite *BuildNodeXnameTestSuite) TestInvalidHardware() {
	topologyNode := TopologyNode{
		CommonName:   "pdu-x3001-000",
		ID:           56,
		Architecture: "pdu",
		Model:        "pdu",
		Type:         "none",
		Vendor:       "hpe",
		Location: Location{
			Rack:      "x3001",
			Elevation: "p0",
		},
	}

	paddle := Paddle{
		Topology: []TopologyNode{topologyNode},
	}

	_, err := BuildNodeXname(topologyNode, paddle, sls_common.ComptypeNode{})
	suite.Errorf(err, "unexpected topology node type (pdu) expected (server or node)")
}

func TestBuildNodeXnameTestSuite(t *testing.T) {
	suite.Run(t, new(BuildNodeXnameTestSuite))
}

type BuildSLSMgmtSwitchConnectorTestSuite struct {
	suite.Suite
}

func (suite *BuildSLSMgmtSwitchConnectorTestSuite) TestIgnore() {
	for _, xname := range []string{"x3000c0w1", "x3000c0h1s1", "d0w1"} {
		hardware, err := BuildSLSMgmtSwitchConnector(sls_common.NewGenericHardware(xname, sls_common.ClassRiver, nil), TopologyNode{}, Paddle{})
		suite.NoError(err)
		suite.Equal(sls_common.GenericHardware{}, hardware)
	}
}

func (suite *BuildSLSMgmtSwitchConnectorTestSuite) TestUnknownSwitchVendor() {
	paddle := Paddle{
		Topology: []TopologyNode{
			// Node
			{
				CommonName:   "uan002",
				ID:           20,
				Architecture: "river_ncn_node_4_port",
				Model:        "river_ncn_node_4_port",
				Type:         "server",
				Vendor:       "hpe",
				Ports: []Port{
					{
						Port:       1,
						Speed:      1,
						Slot:       "bmc",
						DestNodeID: 19,
						DestPort:   41,
					},
				},
				Location: Location{
					Rack:      "x3000",
					Elevation: "u16",
				},
			},

			// Switch
			{
				CommonName:   "sw-leaf-bmc-001",
				ID:           19,
				Architecture: "river_bmc_leaf",
				Model:        "unknown",
				Type:         "switch",
				Vendor:       "unknown",
				Location: Location{
					Rack:      "x3000",
					Elevation: "u31",
				},
			},
		},
	}

	_, err := BuildSLSMgmtSwitchConnector(
		sls_common.NewGenericHardware("x3000c0s16b0n0", sls_common.ClassRiver, nil),
		paddle.Topology[0],
		paddle,
	)
	suite.EqualError(err, "unexpected switch vendor (unknown)")
}

func (suite *BuildSLSMgmtSwitchConnectorTestSuite) TestRouterBMC_Aruba() {

}

func (suite *BuildSLSMgmtSwitchConnectorTestSuite) TestRouterBMC_Dell() {

}

func (suite *BuildSLSMgmtSwitchConnectorTestSuite) TestPDU_Aruba() {

}

func (suite *BuildSLSMgmtSwitchConnectorTestSuite) TestPDU_Dell() {

}

func (suite *BuildSLSMgmtSwitchConnectorTestSuite) TestNode_Aruba() {
	paddle := Paddle{
		Topology: []TopologyNode{
			// Node
			{
				CommonName:   "uan002",
				ID:           20,
				Architecture: "river_ncn_node_4_port",
				Model:        "river_ncn_node_4_port",
				Type:         "server",
				Vendor:       "hpe",
				Ports: []Port{
					{
						Port:       1,
						Speed:      1,
						Slot:       "bmc",
						DestNodeID: 19,
						DestPort:   41,
					},
				},
				Location: Location{
					Rack:      "x3000",
					Elevation: "u16",
				},
			},

			// Switch
			{
				CommonName:   "sw-leaf-bmc-001",
				ID:           19,
				Architecture: "river_bmc_leaf",
				Model:        "6300M_JL762A",
				Type:         "switch",
				Vendor:       "aruba",
				Location: Location{
					Rack:      "x3000",
					Elevation: "u31",
				},
			},
		},
	}

	mgmtSwitchConnector, err := BuildSLSMgmtSwitchConnector(
		sls_common.NewGenericHardware("x3000c0s16b0n0", sls_common.ClassRiver, nil),
		paddle.Topology[0],
		paddle,
	)
	suite.NoError(err)

	expectedMgmtSwitchConnector := sls_common.NewGenericHardware("x3000c0w31j41", sls_common.ClassRiver, sls_common.ComptypeMgmtSwitchConnector{
		VendorName: "1/1/41",
		NodeNics:   []string{"x3000c0s16b0"},
	})

	suite.Equal(expectedMgmtSwitchConnector, mgmtSwitchConnector)
}

func (suite *BuildSLSMgmtSwitchConnectorTestSuite) TestNode_Dell() {
	paddle := Paddle{
		Topology: []TopologyNode{
			// Node
			{
				CommonName:   "uan002",
				ID:           20,
				Architecture: "river_ncn_node_4_port",
				Model:        "river_ncn_node_4_port",
				Type:         "server",
				Vendor:       "hpe",
				Ports: []Port{
					{
						Port:       1,
						Speed:      1,
						Slot:       "bmc",
						DestNodeID: 19,
						DestPort:   41,
					},
				},
				Location: Location{
					Rack:      "x3000",
					Elevation: "u16",
				},
			},

			// Switch
			{
				CommonName:   "sw-leaf-bmc-001",
				ID:           19,
				Architecture: "river_bmc_leaf",
				Model:        "S3048-ON",
				Type:         "switch",
				Vendor:       "dell",
				Location: Location{
					Rack:      "x3000",
					Elevation: "u31",
				},
			},
		},
	}

	mgmtSwitchConnector, err := BuildSLSMgmtSwitchConnector(
		sls_common.NewGenericHardware("x3000c0s16b0n0", sls_common.ClassRiver, nil),
		paddle.Topology[0],
		paddle,
	)
	suite.NoError(err)

	expectedMgmtSwitchConnector := sls_common.NewGenericHardware("x3000c0w31j41", sls_common.ClassRiver, sls_common.ComptypeMgmtSwitchConnector{
		VendorName: "ethernet1/1/41",
		NodeNics:   []string{"x3000c0s16b0"},
	})

	suite.Equal(expectedMgmtSwitchConnector, mgmtSwitchConnector)
}

func TestBuildSLSMgmtSwitchConnector(t *testing.T) {
	suite.Run(t, new(BuildSLSMgmtSwitchConnectorTestSuite))
}
