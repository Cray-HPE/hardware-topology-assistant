package bss

import (
	"net"
	"testing"

	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
)

// simple test data with an NCN of each type
var ncns = []sls_common.GenericHardware{
	{
		Parent:     "x3700c0s1b0",
		Xname:      "x3700c0s1b0n0",
		Type:       "comptype_node",
		Class:      "River",
		TypeString: "Node",
		ExtraPropertiesRaw: map[string]interface{}{
			"Aliases": []string{"ncn-m001"},
			"Role":    "Management",
			"SubRole": "Master",
			"NID":     100001,
		},
	},
	{
		Parent:     "x3700c0s1b0",
		Xname:      "x3700c0s2b0n0",
		Type:       "comptype_node",
		Class:      "River",
		TypeString: "Node",
		ExtraPropertiesRaw: map[string]interface{}{
			"Aliases": []string{"ncn-w001"},
			"Role":    "Management",
			"SubRole": "Worker",
			"NID":     100002,
		},
	},
	{
		Parent:     "x3700c0s1b0",
		Xname:      "x3700c0s3b0n0",
		Type:       "comptype_node",
		Class:      "River",
		TypeString: "Node",
		ExtraPropertiesRaw: map[string]interface{}{
			"Aliases": []string{"ncn-s001"},
			"Role":    "Management",
			"SubRole": "Storage",
			"NID":     100003,
		},
	},
}

var networks = sls_common.NetworkArray{
	{
		Name:     "CHN",
		FullName: "Customer High-Speed Network",
		IPRanges: []string{"10.121.60.0/21"},
		ExtraPropertiesRaw: interface{}(map[string]interface{}{
			"CIDR": "10.121.60.0/21",
			"MTU":  9000,
			"Subnets": []sls_common.IPV4Subnet{
				{
					CIDR:           "10.121.60.0/21",
					Gateway:        net.IPv4(10, 121, 60, 1),
					IPReservations: []sls_common.IPReservation{}, // when no reservations, the function should not fail since CHN is not used for cloud init routes
				},
			},
		}),
	},
	{
		Name:     "HMN",
		FullName: "Hardware Management Network",
		IPRanges: []string{"10.254.0.0/24"},
		ExtraPropertiesRaw: interface{}(map[string]interface{}{
			"CIDR": "10.254.0.0/24",
			"MTU":  9000,
			"Subnets": []sls_common.IPV4Subnet{
				{
					CIDR:    "10.254.20.0/24",
					Gateway: net.IPv4(10, 254, 0, 1),
					IPReservations: []sls_common.IPReservation{
						{Name: "ncn-m001", Comment: "x3700c0s1b0n0", IPAddress: net.IPv4(10, 254, 0, 2)},
						{Name: "ncn-w001", Comment: "x3700c0s2b0n0", IPAddress: net.IPv4(10, 254, 0, 3)},
						{Name: "ncn-s001", Comment: "x3700c0s3b0n0", IPAddress: net.IPv4(10, 254, 0, 4)},
					},
				},
			},
		}),
	},
	{
		Name:     "CMN",
		FullName: "Customer Management Network",
		IPRanges: []string{"10.94.100.0/24"},
		ExtraPropertiesRaw: interface{}(map[string]interface{}{
			"CIDR": "10.94.100.0/24",
			"MTU":  9000,
			"Subnets": []sls_common.IPV4Subnet{
				{
					CIDR:    "10.94.100.0/24",
					Gateway: net.IPv4(10, 94, 100, 1),
					IPReservations: []sls_common.IPReservation{
						{Name: "ncn-m001", Comment: "x3700c0s1b0n0", IPAddress: net.IPv4(10, 94, 100, 2)},
						{Name: "ncn-w001", Comment: "x3700c0s2b0n0", IPAddress: net.IPv4(10, 94, 100, 3)},
						{Name: "ncn-s001", Comment: "x3700c0s3b0n0", IPAddress: net.IPv4(10, 94, 100, 4)},
					},
				},
			},
		}),
	},
	{
		Name:     "MTL",
		FullName: "Provisioning Network (untagged)",
		IPRanges: []string{"10.1.1.0/16"},
		ExtraPropertiesRaw: interface{}(map[string]interface{}{
			"CIDR": "10.1.1.0/16",
			"MTU":  9000,
			"Subnets": []sls_common.IPV4Subnet{
				{
					CIDR:    "10.1.0.0/16",
					Gateway: net.IPv4(10, 1, 1, 1),
					IPReservations: []sls_common.IPReservation{
						{Name: "ncn-m001", Comment: "x3700c0s1b0n0", IPAddress: net.IPv4(10, 1, 1, 2)},
						{Name: "ncn-w001", Comment: "x3700c0s2b0n0", IPAddress: net.IPv4(10, 1, 1, 3)},
						{Name: "ncn-s001", Comment: "x3700c0s3b0n0", IPAddress: net.IPv4(10, 1, 1, 4)},
					},
				},
			},
		}),
	},
	{
		Name:     "NMN",
		FullName: "Provisioning Network (untagged)",
		IPRanges: []string{"10.252.0.0/24"},
		ExtraPropertiesRaw: interface{}(map[string]interface{}{
			"CIDR": "10.252.0.0/24",
			"MTU":  9000,
			"Subnets": []sls_common.IPV4Subnet{
				{
					CIDR:    "10.252.0.0/24",
					Gateway: net.IPv4(10, 252, 0, 1),
					IPReservations: []sls_common.IPReservation{
						{Name: "ncn-m001", Comment: "x3700c0s1b0n0", IPAddress: net.IPv4(10, 252, 0, 2)},
						{Name: "ncn-w001", Comment: "x3700c0s2b0n0", IPAddress: net.IPv4(10, 252, 0, 3)},
						{Name: "ncn-s001", Comment: "x3700c0s3b0n0", IPAddress: net.IPv4(10, 252, 0, 4)},
					},
				},
			},
		}),
	},
}

func TestGetIPAMForNCN(t *testing.T) {
	tests := []struct {
		name             string
		extraSLSNetworks []string
		expectedNetworks map[string]IPAMNetwork
	}{
		{
			name:             "Do not fail on subnet/reservation when CHN is present and has no reservations",
			extraSLSNetworks: []string{"chn"}, // CHN should be skipped and not fail
			expectedNetworks: map[string]IPAMNetwork{
				"cmn": {Gateway: "10.94.100.1", CIDR: "10.94.100.0/24", ParentDevice: "bond0", VlanID: 0},
				"hmn": {Gateway: "10.254.0.1", CIDR: "10.254.0.0/24", ParentDevice: "bond0", VlanID: 0},
				"mtl": {Gateway: "10.1.1.1", CIDR: "10.1.1.0/16", ParentDevice: "bond0", VlanID: 0},
				"nmn": {Gateway: "10.252.0.1", CIDR: "10.252.0.0/24", ParentDevice: "bond0", VlanID: 0},
			},
		},
	}

	for _, tt := range tests {
		for _, ncn := range ncns { // check various types of ncns
			t.Run(tt.name, func(t *testing.T) {
				// run the function
				ipamNetworks := GetIPAMForNCN(ncn, networks, tt.extraSLSNetworks...)

				// fail if there are not enough reservations
				if len(ipamNetworks) != len(tt.expectedNetworks) {
					t.Errorf("expected %d reservations, got %d", len(tt.expectedNetworks), len(ipamNetworks))
				}

				// check if it chose the right network
				for name, ipamNetwork := range ipamNetworks {
					if tt.expectedNetworks[name].Gateway != ipamNetwork.Gateway {
						t.Errorf("expected gateway %s, got %s", tt.expectedNetworks[name].Gateway, ipamNetwork.Gateway)
					}
				}
			})
		}
	}
}
