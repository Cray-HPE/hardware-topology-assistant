package ipam

import (
	"encoding/binary"
	"fmt"
	"math"

	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/Cray-HPE/hms-xname/xnames"
	"inet.af/netaddr"
)

func FindNextAvailableIP(slsSubnet sls_common.IPV4Subnet) (netaddr.IP, error) {
	subnet, err := netaddr.ParseIPPrefix(slsSubnet.CIDR)
	if err != nil {
		return netaddr.IP{}, fmt.Errorf("failed to parse subnet CIDR (%v): %w", slsSubnet.CIDR, err)
	}

	// Debug
	// fmt.Println(subnet)
	// fmt.Println(subnet.Range())

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

	// Debug
	// fmt.Println(existingIPAddressesSet.Ranges())

	startingIP := subnet.Range().From().Next() // Start at the first usable available IP in the subnet.
	endingIP := subnet.Range().To()            // This is the broadcast IP

	for ip := startingIP; ip.Less(endingIP); ip = ip.Next() {
		if !existingIPAddressesSet.Contains(ip) {
			// Debug
			// fmt.Println(ip, "not allocated")
			return ip, nil
		}

		// Debug
		// fmt.Println(ip, "already allocated")
	}

	return netaddr.IP{}, fmt.Errorf("subnet has no available IPs")
}

func AdvanceIP(ip netaddr.IP, n uint32) (netaddr.IP, error) {
	if ip.Is6() {
		return netaddr.IP{}, fmt.Errorf("IPv6 is not supported")
	}
	if ip.IsZero() {
		return netaddr.IP{}, fmt.Errorf("empty IP address provided")
	}

	// This is kind of crude hack, but if it works it works.
	ipOctets := ip.As4()
	ipRaw := binary.BigEndian.Uint32(ipOctets[:])

	// Advance the IP by n
	ipRaw += n

	// Now put it back into an netaddr.IP
	var updatedIPOctets [4]byte
	binary.BigEndian.PutUint32(updatedIPOctets[:], ipRaw)

	return netaddr.IPFrom4(updatedIPOctets), nil
}

func SplitNetwork(network netaddr.IPPrefix, subnetMaskOneBits uint8) ([]netaddr.IPPrefix, error) {
	if subnetMaskOneBits < 16 || 30 < subnetMaskOneBits {
		return nil, fmt.Errorf("invalid subnet mask provided /%d", subnetMaskOneBits)
	}

	subnetStartIP := network.Range().From()

	// TODO add a counter to prevent this loop from going in forever!
	var subnets []netaddr.IPPrefix
	for {
		subnets = append(subnets, netaddr.IPPrefixFrom(subnetStartIP, subnetMaskOneBits))

		advanceBy := uint32(math.Pow(2, float64(32-subnetMaskOneBits)))

		// Now advance!
		var err error
		subnetStartIP, err = AdvanceIP(subnetStartIP, advanceBy)
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

	// Debug
	// fmt.Println("Network IP Address range", existingSubnetsSet.Ranges())

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
			// Debug
			// fmt.Println(subnet, "-", subnet.Range(), "Taken")
			continue
		}
		// Debug
		// fmt.Println(subnet, "-", subnet.Range(), "is free!")
		return subnet, nil
	}

	return netaddr.IPPrefix{}, fmt.Errorf("network space has been exhausted")
}

func AllocateCabinetSubnet(slsNetwork sls_common.NetworkExtraProperties, xname xnames.Cabinet, vlanOverride *int16) (sls_common.IPV4Subnet, error) {
	cabinetSubnet, err := FindNextAvailableSubnet(slsNetwork)
	if err != nil {
		return sls_common.IPV4Subnet{}, fmt.Errorf("failed to allocate subnet for (%s) in CIDR (%s)", xname.String(), slsNetwork.CIDR)
	}

	// Verify this subnet is new
	subnetName := fmt.Sprintf("cabinet_%d", xname.Cabinet)
	for _, otherSubnet := range slsNetwork.Subnets {
		if otherSubnet.Name == subnetName {
			return sls_common.IPV4Subnet{}, fmt.Errorf("subnet (%s) already exists", subnetName)
		}
	}

	// Calculate VLAN if one was not provided
	vlan := int16(-1)
	if vlanOverride != nil {
		vlan = *vlanOverride
	} else {
		// Look at other cabinets in the subnet and pick one.
		// TODO THIS MIGHT FALL APART WITH LIQUID-COOLED CABINETS AS THOSE CAN BE USER SUPPLIED
	}
	// TODO make sure vlan is unique

	// DHCP starts 10 into the subnet
	dhcpStart, err := AdvanceIP(cabinetSubnet.Range().From(), 10)
	if err != nil {
		return sls_common.IPV4Subnet{}, fmt.Errorf("failed to determine DHCP start in CIDR (%s)", cabinetSubnet.String())
	}

	return sls_common.IPV4Subnet{
		Name:      subnetName,
		CIDR:      cabinetSubnet.String(),
		VlanID:    vlan,
		Gateway:   cabinetSubnet.Range().From().Next().IPAddr().IP,
		DHCPStart: dhcpStart.IPAddr().IP,
		DHCPEnd:   cabinetSubnet.Range().To().Prior().IPAddr().IP,
	}, nil
}

func AllocateIP(slsSubnet sls_common.IPV4Subnet, xname xnames.Xname, alias string) (sls_common.IPReservation, error) {
	ip, err := FindNextAvailableIP(slsSubnet)
	if err != nil {
		return sls_common.IPReservation{}, fmt.Errorf("failed to allocate ip for switch (%s) in subnet (%s)", xname.String(), slsSubnet.CIDR)
	}

	// Verify this switch is unique within the subnet
	for _, ipReservation := range slsSubnet.IPReservations {
		if ipReservation.Name == alias {
			return sls_common.IPReservation{}, fmt.Errorf("ip reservation with name (%v) already exits", alias)
		}

		if ipReservation.Comment == xname.String() {
			return sls_common.IPReservation{}, fmt.Errorf("ip reservation with xname (%v) already exits", xname.String())
		}
	}

	// TODO Move this outside this function? So this function just gives back IP within the subnet, and then have outside logic
	// Verify that the IP is actually valid ie within the DHCP range, and if not in the DHCP range expand it and verify nothing is
	// using the IP address.

	// Verify IP is within the static IP range
	// Debug
	// fmt.Println("DHCP Start", slsSubnet.DHCPStart)
	if slsSubnet.DHCPStart != nil {
		// Debug
		// fmt.Println("Enforcing static IP address range")
		dhcpStart, ok := netaddr.FromStdIP(slsSubnet.DHCPStart)
		if !ok {
			return sls_common.IPReservation{}, fmt.Errorf("failed to convert DHCP Start IP address to netaddr struct")
		}

		if !ip.Less(dhcpStart) {
			return sls_common.IPReservation{}, fmt.Errorf("ip reservation with xname (%v) and IP %s is outside the static IP address range - starting DHCP IP is %s", xname.String(), ip.String(), slsSubnet.DHCPStart.String())
		}
	} else {
		// Debug
		// fmt.Println("No DHCP range")
	}

	return sls_common.IPReservation{
		Comment:   xname.String(),
		IPAddress: ip.IPAddr().IP,
		Name:      alias,
	}, nil
}

func FreeIPsInStaticRange(slsSubnet sls_common.IPV4Subnet) (uint32, error) {
	// TODO add the functionality for this
	// Probably need to steal some of the logic for allocate IP. Need to share the logic between the two
	return 0, nil
}

func ExpandSubnetStaticRange(slsSubnet *sls_common.IPV4Subnet, count uint32) error {
	if slsSubnet.DHCPStart == nil || slsSubnet.DHCPEnd == nil {
		return fmt.Errorf("subnet does not have DHCP range")
	}

	dhcpStart, ok := netaddr.FromStdIP(slsSubnet.DHCPStart)
	if !ok {
		return fmt.Errorf("failed to convert DHCP Start IP address to netaddr struct")
	}

	dhcpEnd, ok := netaddr.FromStdIP(slsSubnet.DHCPEnd)
	if !ok {
		return fmt.Errorf("failed to convert DHCP END IP address to netaddr struct")
	}

	// Move it forward!
	dhcpStart, err := AdvanceIP(dhcpStart, count)
	if err != nil {
		return fmt.Errorf("failed to advice DHCP Start IP address: %w", err)
	}

	// Verify the DHCP Start address is smaller than the end address
	if !dhcpStart.Less(dhcpEnd) {
		return fmt.Errorf("new DHCP Start address %v is equal or larger then the DHCP End address %v", dhcpStart, dhcpEnd)
	}

	// Now update the SLS subnet
	slsSubnet.DHCPStart = dhcpStart.IPAddr().IP
	return nil
}
