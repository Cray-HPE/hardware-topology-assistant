package ipam

import (
	"fmt"

	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"inet.af/netaddr"
)

func FindNextAvailableIP(slsSubnet sls_common.IPV4Subnet) (*netaddr.IP, error) {
	subnet, err := netaddr.ParseIPPrefix(slsSubnet.CIDR)
	if err != nil {
		return nil, fmt.Errorf("failed to parse subnet CIDR (%v): %w", slsSubnet.CIDR, err)
	}

	fmt.Println(subnet)
	fmt.Println(subnet.Range())

	var existingIPAddresses netaddr.IPSetBuilder
	gatewayIP, ok := netaddr.FromStdIP(slsSubnet.Gateway)
	if !ok {
		return nil, fmt.Errorf("failed to parse gateway IP (%v)", slsSubnet.Gateway)
	}
	existingIPAddresses.Add(gatewayIP)

	for _, ipReservation := range slsSubnet.IPReservations {
		ip, ok := netaddr.FromStdIP(ipReservation.IPAddress)
		if !ok {
			return nil, fmt.Errorf("failed to parse IPReservation IP (%v)", ipReservation.IPAddress)
		}
		existingIPAddresses.Add(ip)
	}

	existingIPAddressesSet, err := existingIPAddresses.IPSet()
	if err != nil {
		panic(err)
	}
	fmt.Println(existingIPAddressesSet.Ranges())

	startingIP := subnet.Range().From().Next() // Start at the first usable available IP in the subnet.
	endingIP := subnet.Range().To()            // This is the broadcast IP

	for ip := startingIP; ip.Less(endingIP); ip = ip.Next() {
		if !existingIPAddressesSet.Contains(ip) {
			fmt.Println(ip, "not allocated")
			return &ip, nil
		}

		fmt.Println(ip, "already allocated")
	}

	return nil, fmt.Errorf("subnet has no available IPs")
}
