package bss

import (
	"fmt"
	"log"
	"net"
	"sort"
	"strings"

	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/Cray-HPE/hms-xname/xnametypes"
	"github.com/mitchellh/mapstructure"
	"github.hpe.com/sjostrand/topology-tool/pkg/sls"
)

//
// TODO look into making this package return an error
//

// This following code was taken from CSI
// https://github.com/Cray-HPE/cray-site-init/blob/main/cmd/upgrade-metadata.go#L294-L422

func GetIPAMForNCN(managementNCN sls_common.GenericHardware,
	networks sls_common.NetworkArray, extraSLSNetworks ...string) (ipamNetworks CloudInitIPAM) {
	ipamNetworks = make(CloudInitIPAM)

	// For each of the required networks, go build an IPAMNetwork object and add that to the ipamNetworks
	// above.
	for _, ipamNetwork := range append(IPAMNetworks[:], extraSLSNetworks...) {
		// Search SLS networks for this network.
		var targetSLSNetwork *sls_common.Network
		for _, slsNetwork := range networks {
			if strings.ToLower(slsNetwork.Name) == ipamNetwork {
				targetSLSNetwork = &slsNetwork
				break
			}
		}

		if targetSLSNetwork == nil {
			log.Fatalf("Failed to find required IPAM network [%s] in SLS networks!", ipamNetwork)
		}

		// Map this network to a usable structure.
		var networkExtraProperties sls_common.NetworkExtraProperties
		err := sls.DecodeNetworkExtraProperties(targetSLSNetwork.ExtraPropertiesRaw, &networkExtraProperties)
		if err != nil {
			log.Fatalf("Failed to decode raw network extra properties to correct structure: %s", err)
		}

		// The target SLS network is determined, now we need the right reservation.
		var targetSubnet *sls_common.IPV4Subnet
		var targetReservation *sls_common.IPReservation

		_, targetNet, err := net.ParseCIDR(networkExtraProperties.CIDR)
		if err != nil {
			log.Fatalf("Failed to parse SLS network CIDR (%s): %s", networkExtraProperties.CIDR, err)
		}

		for _, subnet := range networkExtraProperties.Subnets {
			for _, reservation := range subnet.IPReservations {
				if reservation.Comment == managementNCN.Xname {
					targetSubnet = &subnet
					targetReservation = &reservation

					break
				}
			}

			if targetSubnet != nil {
				break
			}
		}

		if targetSubnet == nil || targetReservation == nil {
			log.Fatalf("Failed to find subnet/reservation for this managment NCN xname (%s)!",
				managementNCN.Xname)
		}

		// Finally, we have all the pieces, wrangle the data! Speaking of, here's an example of what this
		// should look like after we're done:
		//  "can": {
		//   "gateway": "10.10.0.1",
		//   "ip": "10.10.0.1/26",
		//   "parent_device": "bond0",
		//   "vlanid": 999
		//  }
		// Few things to note, the `ip` field is a bit of a misnomer as it also must include the mask bits.
		// Which is our first step, figure out just the mask bits from the subnet's CIDR. One might be
		// tempted to just use string splits and take the 1st element, but we get validation for free this
		// way.

		_, ipv4Net, err := net.ParseCIDR(targetSubnet.CIDR)
		if err != nil {
			log.Fatalf("Failed to parse SLS network CIDR (%s): %s", targetSubnet.CIDR, err)
		}

		var maskBits int
		if ipamNetwork == "cmn" || ipamNetwork == "can" {
			maskBits, _ = targetNet.Mask.Size()
		} else {
			maskBits, _ = ipv4Net.Mask.Size()
		}

		// Now we can build an IPAM object.
		thisIPAMNetwork := IPAMNetwork{
			Gateway:      targetSubnet.Gateway.String(),
			CIDR:         fmt.Sprintf("%s/%d", targetReservation.IPAddress, maskBits),
			ParentDevice: "bond0", // FIXME: Remove bond0 hardcode.
			VlanID:       targetSubnet.VlanID,
		}
		ipamNetworks[ipamNetwork] = thisIPAMNetwork
	}

	return
}

func GetWriteFiles(networks sls_common.NetworkArray, ipamNetworks CloudInitIPAM) (writeFiles []WriteFile) {
	// In the case of 1.0 -> 1.2 we need to add route files for a few of the networks.
	// The process is simple, get the CIDR and gateway for those networks and then format them as an ifroute file.
	// Here's an example:
	routeFiles := make(map[string][]string)

	for _, neededNetwork := range []string{"cmn", "hmn", "nmn"} {
		ipamNetwork := ipamNetworks[neededNetwork]

		for _, network := range networks {
			// We have to check the prefix of the network because some networks like the NMN or HMN also have LB
			// networks associated with them. Debatable whether that actually make them a separate network
			// or not but the important point here is they have to be added to the correct file.
			// We also need to add a route for the MTL network to the NMN gateway
			if strings.HasPrefix(strings.ToLower(network.Name), neededNetwork) ||
				(neededNetwork == "nmn" && strings.ToLower(network.Name) == "mtl") {
				// Map this network to a usable structure.
				var networkExtraProperties sls_common.NetworkExtraProperties
				err := sls.DecodeNetworkExtraProperties(network.ExtraPropertiesRaw, &networkExtraProperties)
				if err != nil {
					log.Fatalf("Failed to decode raw network extra properties to correct structure: %s", err)
				}

				thisRouteFile := routeFiles[neededNetwork]

				// Now we know we need to add this network, go through all the subnets and build up the route file.
				for _, subnet := range networkExtraProperties.Subnets {
					_, ipv4Net, err := net.ParseCIDR(subnet.CIDR)
					if err != nil {
						log.Fatalf("Failed to parse SLS network CIDR (%s): %s", subnet.CIDR, err)
					}

					// If the gateway fits in the CIDR then we don't need it, the OS will give us that for free.
					gatewayIP := net.ParseIP(ipamNetwork.Gateway)
					if ipv4Net.Contains(gatewayIP) {
						continue
					}

					route := fmt.Sprintf("%s %s - %s.%s0",
						ipv4Net.String(), gatewayIP.String(), "bond0", neededNetwork)

					// Don't add the route if we already have it
					found := false
					for _, a := range thisRouteFile {
						if a == route {
							found = true
							break
						}
					}

					if !found {
						thisRouteFile = append(thisRouteFile, route)
					}
				}

				// Sort the routes so the order of them is deterministic
				sort.Strings(thisRouteFile)

				routeFiles[neededNetwork] = thisRouteFile
			}
		}
	}

	// Sort the network names so the output is deterministic
	var networkNames []string
	for networkName := range routeFiles {
		networkNames = append(networkNames, networkName)
	}
	sort.Strings(networkNames)

	// We now have all the write files, let's make objects for them.
	for _, networkName := range networkNames {
		routeFile := routeFiles[networkName]

		writeFile := WriteFile{
			Content:     strings.Join(routeFile, "\n"),
			Owner:       "root:root",
			Path:        fmt.Sprintf("/etc/sysconfig/network/ifroute-bond0.%s0", networkName),
			Permissions: "0644",
		}
		writeFiles = append(writeFiles, writeFile)
	}

	return
}

// buildBSSHostRecord will build a BSS HostRecord
func BuildBSSHostRecord(networkEPs map[string]*sls_common.NetworkExtraProperties, networkName, subnetName, reservationName string, aliases []string) HostRecord {
	subnet, _, err := networkEPs[networkName].LookupSubnet(subnetName)
	if err != nil {
		log.Fatalf("Unable to find %s in the %s network", subnetName, networkName)
	}
	ipReservation, found := subnet.ReservationsByName()[reservationName]
	if !found {
		log.Fatalf("Failed to find IP reservation for %s in the %s %s subnet", reservationName, networkName, subnetName)
	}

	return HostRecord{
		IP:      ipReservation.IPAddress.String(),
		Aliases: aliases,
	}
}

// getBSSGlobalHostRecords is the BSS analog of the pit.MakeBasecampHostRecords that works with SLS data
func GetBSSGlobalHostRecords(managementNCNs []sls_common.GenericHardware, networks sls_common.NetworkArray) HostRecords {

	// Collase all of the Network ExtraProperties into single map for lookups
	networkEPs := map[string]*sls_common.NetworkExtraProperties{}
	for _, network := range networks {
		// Map this network to a usable structure.
		var networkExtraProperties sls_common.NetworkExtraProperties
		err := sls.DecodeNetworkExtraProperties(network.ExtraPropertiesRaw, &networkExtraProperties)
		if err != nil {
			log.Fatalf("Failed to decode raw network extra properties to correct structure: %s", err)
		}

		networkEPs[network.Name] = &networkExtraProperties
	}

	var globalHostRecords HostRecords

	// Add the NCN Interfaces.
	for _, managementNCN := range managementNCNs {
		var ncnExtraProperties sls_common.ComptypeNode
		err := mapstructure.Decode(managementNCN.ExtraPropertiesRaw, &ncnExtraProperties)
		if err != nil {
			log.Fatalf("Failed to decode raw NCN extra properties to correct structure: %s", err)
		}

		if len(ncnExtraProperties.Aliases) == 0 {
			log.Fatalf("NCN has no aliases defined in SLS: %+v", managementNCN)
		}

		ncnAlias := ncnExtraProperties.Aliases[0]

		// Add the NCN interface host records.
		var ipamNetworks CloudInitIPAM
		extraNets := []string{}

		if _, ok := networkEPs["CHN"]; ok {
			extraNets = append(extraNets, "chn")
		}
		if _, ok := networkEPs["CAN"]; ok {
			extraNets = append(extraNets, "can")
		}

		if len(extraNets) == 0 {
			log.Fatalf("SLS must have either CAN or CHN defined")
		}
		ipamNetworks = GetIPAMForNCN(managementNCN, networks, extraNets...)

		for network, ipam := range ipamNetworks {
			// Get the IP of the NCN for this network.
			ip, _, err := net.ParseCIDR(ipam.CIDR)
			if err != nil {
				log.Fatalf("Failed to parse BSS IPAM Network CIDR (%s): %s", ipam.CIDR, err)
			}

			hostRecord := HostRecord{
				IP:      ip.String(),
				Aliases: []string{fmt.Sprintf("%s.%s", ncnAlias, network)},
			}

			// The NMN network gets the privilege of also containing the bare NCN Alias without network domain.
			if strings.ToLower(network) == "nmn" {
				hostRecord.Aliases = append(hostRecord.Aliases, ncnAlias)
			}
			globalHostRecords = append(globalHostRecords, hostRecord)
		}

		// Next add the NCN BMC host record
		bmcXname := xnametypes.GetHMSCompParent(managementNCN.Xname)
		globalHostRecords = append(globalHostRecords,
			BuildBSSHostRecord(networkEPs, "HMN", "bootstrap_dhcp", bmcXname, []string{fmt.Sprintf("%s-mgmt", ncnAlias)}),
		)
	}

	// Add kubeapi-vip
	globalHostRecords = append(globalHostRecords,
		BuildBSSHostRecord(networkEPs, "NMN", "bootstrap_dhcp", "kubeapi-vip", []string{"kubeapi-vip", "kubeapi-vip.nmn"}),
	)

	// Add rgw-vip
	globalHostRecords = append(globalHostRecords,
		BuildBSSHostRecord(networkEPs, "NMN", "bootstrap_dhcp", "rgw-vip", []string{"rgw-vip", "rgw-vip.nmn"}),
	)

	// Using the original InstallNCN as the host for pit.nmn
	// HACK, I'm assuming ncn-m001
	globalHostRecords = append(globalHostRecords,
		BuildBSSHostRecord(networkEPs, "NMN", "bootstrap_dhcp", "ncn-m001", []string{"pit", "pit.nmn"}),
	)

	// Add in packages.local and registry.local pointing toward the API Gateway
	globalHostRecords = append(globalHostRecords,
		BuildBSSHostRecord(networkEPs, "NMNLB", "nmn_metallb_address_pool", "istio-ingressgateway", []string{"packages.local", "registry.local"}),
	)

	// Add entries for switches
	hmnNetSubnet, _, err := networkEPs["HMN"].LookupSubnet("network_hardware")
	if err != nil {
		log.Fatal("Unable to find network_hardware in the HMN network")
	}

	for _, ipReservation := range hmnNetSubnet.IPReservations {
		if strings.HasPrefix(ipReservation.Name, "sw-") {
			globalHostRecords = append(globalHostRecords, HostRecord{
				IP:      ipReservation.IPAddress.String(),
				Aliases: []string{ipReservation.Name},
			})
		}
	}

	// Sort the records so the order of the host records is deterministic
	sort.SliceStable(globalHostRecords, func(i, j int) bool {
		return globalHostRecords[i].IP < globalHostRecords[j].IP
	})

	return globalHostRecords
}
