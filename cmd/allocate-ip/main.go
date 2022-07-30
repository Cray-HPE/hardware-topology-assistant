package main

import (
	"encoding/json"
	"fmt"
	"log"

	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.hpe.com/sjostrand/topology-tool/pkg/ipam"
	"github.hpe.com/sjostrand/topology-tool/pkg/sls"
)

var canNetworkRaw = `
{
	"Name": "CAN",
	"FullName": "Customer Access Network",
	"IPRanges": [
	  "10.102.97.128/25"
	],
	"Type": "ethernet",
	"LastUpdated": 1655153192,
	"LastUpdatedTime": "2022-06-13 20:46:32.255491 +0000 +0000",
	"ExtraProperties": {
	  "CIDR": "10.102.97.128/25",
	  "MTU": 9000,
	  "Subnets": [
		{
		  "CIDR": "10.102.97.128/25",
		  "DHCPEnd": "10.102.97.190",
		  "DHCPStart": "10.102.97.144",
		  "FullName": "CAN Bootstrap DHCP Subnet",
		  "Gateway": "10.102.97.129",
		  "IPReservations": [
			{
			  "IPAddress": "10.102.97.130",
			  "Name": "can-switch-1"
			},
			{
			  "IPAddress": "10.102.97.131",
			  "Name": "can-switch-2"
			},
			{
			  "Aliases": [
				"ncn-s001-can",
				"time-can",
				"time-can.local"
			  ],
			  "Comment": "x3000c0s17b0n0",
			  "IPAddress": "10.102.97.132",
			  "Name": "ncn-s001"
			},
			{
			  "Aliases": [
				"ncn-s002-can",
				"time-can",
				"time-can.local"
			  ],
			  "Comment": "x3000c0s19b0n0",
			  "IPAddress": "10.102.97.133",
			  "Name": "ncn-s002"
			},
			{
			  "Aliases": [
				"ncn-s003-can",
				"time-can",
				"time-can.local"
			  ],
			  "Comment": "x3000c0s21b0n0",
			  "IPAddress": "10.102.97.134",
			  "Name": "ncn-s003"
			},
			{
			  "Aliases": [
				"ncn-m001-can",
				"time-can",
				"time-can.local"
			  ],
			  "Comment": "x3000c0s1b0n0",
			  "IPAddress": "10.102.97.135",
			  "Name": "ncn-m001"
			},
			{
			  "Aliases": [
				"ncn-m002-can",
				"time-can",
				"time-can.local"
			  ],
			  "Comment": "x3000c0s3b0n0",
			  "IPAddress": "10.102.97.136",
			  "Name": "ncn-m002"
			},
			{
			  "Aliases": [
				"ncn-m003-can",
				"time-can",
				"time-can.local"
			  ],
			  "Comment": "x3000c0s5b0n0",
			  "IPAddress": "10.102.97.137",
			  "Name": "ncn-m003"
			},
			{
			  "Aliases": [
				"ncn-w001-can",
				"time-can",
				"time-can.local"
			  ],
			  "Comment": "x3000c0s7b0n0",
			  "IPAddress": "10.102.97.138",
			  "Name": "ncn-w001"
			},
			{
			  "Aliases": [
				"ncn-w002-can",
				"time-can",
				"time-can.local"
			  ],
			  "Comment": "x3000c0s9b0n0",
			  "IPAddress": "10.102.97.139",
			  "Name": "ncn-w002"
			},
			{
			  "Aliases": [
				"ncn-w003-can",
				"time-can",
				"time-can.local"
			  ],
			  "Comment": "x3000c0s11b0n0",
			  "IPAddress": "10.102.97.140",
			  "Name": "ncn-w003"
			},
			{
			  "Aliases": [
				"ncn-w004-can",
				"time-can",
				"time-can.local"
			  ],
			  "Comment": "x3000c0s13b0n0",
			  "IPAddress": "10.102.97.141",
			  "Name": "ncn-w004"
			},
			{
			  "Aliases": [
				"ncn-w005-can",
				"time-can",
				"time-can.local"
			  ],
			  "Comment": "x3000c0s25b0n0",
			  "IPAddress": "10.102.97.142",
			  "Name": "ncn-w005"
			},
			{
			  "Comment": "x3000c0s23b0n0",
			  "IPAddress": "10.102.97.143",
			  "Name": "uan01"
			}
		  ],
		  "Name": "bootstrap_dhcp",
		  "VlanID": 6
		},
		{
		  "CIDR": "10.102.97.192/26",
		  "FullName": "CAN Dynamic MetalLB",
		  "Gateway": "10.102.97.193",
		  "MetalLBPoolName": "customer-access",
		  "Name": "can_metallb_address_pool",
		  "VlanID": 6
		}
	  ],
	  "VlanRange": [
		6
	  ]
	}
  }`

// def find_next_available_ip(sls_subnet):
//     subnet = netaddr.IPNetwork(sls_subnet["CIDR"])
//
//     existing_ip_reservations = netaddr.IPSet()
//     existing_ip_reservations.add(sls_subnet["Gateway"])
//     for ip_reservation in sls_subnet["IPReservations"]:
//         print("  Found existing IP reservation {} with IP {}".format(ip_reservation["Name"], ip_reservation["IPAddress"]))
//         existing_ip_reservations.add(ip_reservation["IPAddress"])
//
//     for available_ip in list(subnet[1:-2]):
//         if available_ip not in existing_ip_reservations:
//             print("  {} Available for use.".format(available_ip))
//             return available_ip

func main() {
	var canNetwork sls_common.Network
	if err := json.Unmarshal([]byte(canNetworkRaw), &canNetwork); err != nil {
		panic(err)
	}

	// Map this network to a usable structure.
	var networkExtraProperties sls_common.NetworkExtraProperties
	err := sls.DecodeNetworkExtraProperties(canNetwork.ExtraPropertiesRaw, &networkExtraProperties)
	if err != nil {
		log.Fatalf("Failed to decode raw network extra properties to correct structure: %s", err)
	}

	bootstrapDHCPSubnet, _, err := networkExtraProperties.LookupSubnet("bootstrap_dhcp")
	if err != nil {
		panic(err)
	}

	ip, err := ipam.FindNextAvailableIP(bootstrapDHCPSubnet)
	if err != nil {
		panic(err)
	}

	fmt.Println("Allocated", ip)
}
