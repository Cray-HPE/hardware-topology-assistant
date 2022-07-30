package ipam

import (
	"encoding/binary"
	"fmt"
	"math"

	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"inet.af/netaddr"
)

func FindNextAvailableIP(slsSubnet sls_common.IPV4Subnet) (netaddr.IP, error) {
	subnet, err := netaddr.ParseIPPrefix(slsSubnet.CIDR)
	if err != nil {
		return netaddr.IP{}, fmt.Errorf("failed to parse subnet CIDR (%v): %w", slsSubnet.CIDR, err)
	}

	fmt.Println(subnet)
	fmt.Println(subnet.Range())

	var existingIPAddresses netaddr.IPSetBuilder
	gatewayIP, ok := netaddr.FromStdIP(slsSubnet.Gateway)
	if !ok {
		return netaddr.IP{}, fmt.Errorf("failed to parse gateway IP (%v)", slsSubnet.Gateway)
	}
	existingIPAddresses.Add(gatewayIP)

	for _, ipReservation := range slsSubnet.IPReservations {
		ip, ok := netaddr.FromStdIP(ipReservation.IPAddress)
		if !ok {
			return netaddr.IP{}, fmt.Errorf("failed to parse IPReservation IP (%v)", ipReservation.IPAddress)
		}
		existingIPAddresses.Add(ip)
	}

	existingIPAddressesSet, err := existingIPAddresses.IPSet()
	if err != nil {
		return netaddr.IP{}, err
	}
	fmt.Println(existingIPAddressesSet.Ranges())

	startingIP := subnet.Range().From().Next() // Start at the first usable available IP in the subnet.
	endingIP := subnet.Range().To()            // This is the broadcast IP

	for ip := startingIP; ip.Less(endingIP); ip = ip.Next() {
		if !existingIPAddressesSet.Contains(ip) {
			fmt.Println(ip, "not allocated")
			return ip, nil
		}

		fmt.Println(ip, "already allocated")
	}

	return netaddr.IP{}, fmt.Errorf("subnet has no available IPs")
}

func AdvanceSubnet(ip netaddr.IP, subnetMaskOneBits uint8) (netaddr.IP, error) {
	if ip.Is6() {
		return netaddr.IP{}, fmt.Errorf("IPv6 is not supported")
	}
	if ip.IsZero() {
		return netaddr.IP{}, fmt.Errorf("empty IP address provided")
	}

	if subnetMaskOneBits < 16 || 30 < subnetMaskOneBits {
		return netaddr.IP{}, fmt.Errorf("invalid subnet mask provided /%d", subnetMaskOneBits)
	}

	// This is kind of crude hack, but if it works it works.
	ipOctets := ip.As4()
	ipRaw := binary.BigEndian.Uint32(ipOctets[:])

	// Advance the subnet
	ipRaw += uint32(math.Pow(2, float64(32-subnetMaskOneBits)))

	// Now put it back into an netaddr.IP
	var updatedIPOctets [4]byte
	binary.BigEndian.PutUint32(updatedIPOctets[:], ipRaw)

	return netaddr.IPFrom4(updatedIPOctets), nil
}

func SplitNetwork(network netaddr.IPPrefix, subnetMaskBits uint8) ([]netaddr.IPPrefix, error) {
	var subnets []netaddr.IPPrefix

	subnetStartIP := network.Range().From()

	// TODO add a counter to prevent this loop from going in forever!
	for {
		subnets = append(subnets, netaddr.IPPrefixFrom(subnetStartIP, subnetMaskBits))

		// Now advance!
		var err error
		subnetStartIP, err = AdvanceSubnet(subnetStartIP, subnetMaskBits)
		if err != nil {
			return nil, err
		}

		if network.Range().To().Less(subnetStartIP) {
			break
		}
	}

	return subnets, nil
}

func FindNextAvailableSubnet(slsNetwork sls_common.NetworkExtraProperties) (netaddr.IPPrefix, error) {
	// TODO make the /22 configurable
	var existingSubnets netaddr.IPSetBuilder
	for _, slsSubnet := range slsNetwork.Subnets {
		subnet, err := netaddr.ParseIPPrefix(slsSubnet.CIDR)
		if err != nil {
			return netaddr.IPPrefix{}, fmt.Errorf("failed to parse subnet CIDR (%v): %w", slsSubnet.CIDR, err)
		}

		existingSubnets.AddPrefix(subnet)
	}

	existingSubnetsSet, err := existingSubnets.IPSet()
	if err != nil {
		return netaddr.IPPrefix{}, err
	}

	fmt.Println("Network IP Address rnage", existingSubnetsSet.Ranges())

	network, err := netaddr.ParseIPPrefix(slsNetwork.CIDR)
	if err != nil {
		return netaddr.IPPrefix{}, err
	}

	availableSubnets, err := SplitNetwork(network, 22)
	if err != nil {
		return netaddr.IPPrefix{}, err
	}
	for _, subnet := range availableSubnets {
		if existingSubnetsSet.Contains(subnet.IP()) {
			fmt.Println(subnet, "-", subnet.Range(), "Taken")
			continue
		}
		fmt.Println(subnet, "-", subnet.Range(), "is free!")
		return subnet, nil
	}

	return netaddr.IPPrefix{}, fmt.Errorf("network space has been exhausted")
}
