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

package ipam

import (
	"encoding/binary"
	"fmt"
	"math"

	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/Cray-HPE/hms-xname/xnames"
	"inet.af/netaddr"
)

func ExistingIPAddresses(slsSubnet sls_common.IPV4Subnet) (*netaddr.IPSet, error) {
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

	return existingIPAddresses.IPSet()
}

func FindNextAvailableIP(slsSubnet sls_common.IPV4Subnet) (netaddr.IP, error) {
	subnet, err := netaddr.ParseIPPrefix(slsSubnet.CIDR)
	if err != nil {
		return netaddr.IP{}, fmt.Errorf("failed to parse subnet CIDR (%v): %w", slsSubnet.CIDR, err)
	}

	existingIPAddressesSet, err := ExistingIPAddresses(slsSubnet)
	if err != nil {
		return netaddr.IP{}, err
	}

	startingIP := subnet.Range().From().Next() // Start at the first usable available IP in the subnet.
	endingIP := subnet.Range().To()            // This is the broadcast IP

	for ip := startingIP; ip.Less(endingIP); ip = ip.Next() {
		if !existingIPAddressesSet.Contains(ip) {
			return ip, nil
		}
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
			continue
		}

		return subnet, nil
	}

	return netaddr.IPPrefix{}, fmt.Errorf("network space has been exhausted")
}

func AllocateCabinetSubnet(networkName string, slsNetwork sls_common.NetworkExtraProperties, xname xnames.Cabinet, vlanOverride *int16) (sls_common.IPV4Subnet, error) {
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
		// TODO THIS MIGHT FALL APART WITH LIQUID-COOLED CABINETS AS THOSE CAN BE USER SUPPLIED, but we don't currently support adding this with this tool

		// Determine the current vlans in use by other cabinets
		vlansInUse := map[int16]bool{}
		for _, existingSubnet := range slsNetwork.Subnets {
			vlansInUse[existingSubnet.VlanID] = true
		}

		// Now lest find the smallest free Vlan!
		var vlanLow int16 = -1
		var vlanHigh int16 = -1

		if networkName == "HMN_RVR" {
			// The following values are defined here in CSI: https://github.com/Cray-HPE/cray-site-init/blob/4ead6fccd0ba0710e7250357f1c3a2525996d293/cmd/init.go#L160
			vlanLow = 1513
			vlanHigh = 1769
		} else if networkName == "NMN_RVR" {
			// The following values are defined here in CSI: https://github.com/Cray-HPE/cray-site-init/blob/4ead6fccd0ba0710e7250357f1c3a2525996d293/cmd/init.go#L189
			vlanLow = 1770
			vlanHigh = 1999

		} else {
			return sls_common.IPV4Subnet{}, fmt.Errorf("unknown network (%s) unable to allocate vlan for cabinet subnet", networkName)
		}

		for vlanCandidate := vlanLow; vlanCandidate <= vlanHigh; vlanCandidate++ {
			if vlansInUse[vlanCandidate] {
				// currently in use
				continue
			}

			vlan = vlanCandidate
			break
		}
	}

	if vlan == -1 {
		return sls_common.IPV4Subnet{}, fmt.Errorf("failed to allocate VLAN for cabinet subnet (%s)", subnetName)
	}

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
	if slsSubnet.DHCPStart != nil {
		dhcpStart, ok := netaddr.FromStdIP(slsSubnet.DHCPStart)
		if !ok {
			return sls_common.IPReservation{}, fmt.Errorf("failed to convert DHCP Start IP address to netaddr struct")
		}

		if !ip.Less(dhcpStart) {
			return sls_common.IPReservation{}, fmt.Errorf("ip reservation with xname (%v) and IP %s is outside the static IP address range - starting DHCP IP is %s", xname.String(), ip.String(), slsSubnet.DHCPStart.String())
		}
	}

	return sls_common.IPReservation{
		Comment:   xname.String(),
		IPAddress: ip.IPAddr().IP,
		Name:      alias,
	}, nil
}

func FreeIPsInStaticRange(slsSubnet sls_common.IPV4Subnet) (uint32, error) {
	// Probably need to steal some of the logic for allocate IP. Need to share the logic between the two

	subnet, err := netaddr.ParseIPPrefix(slsSubnet.CIDR)
	if err != nil {
		return 0, fmt.Errorf("failed to parse subnet CIDR (%v): %w", slsSubnet.CIDR, err)
	}

	existingIPAddressesSet, err := ExistingIPAddresses(slsSubnet)
	if err != nil {
		return 0, err
	}

	startingIP := subnet.Range().From().Next() // Start at the first usable available IP in the subnet.
	endingIP, ok := netaddr.FromStdIP(slsSubnet.DHCPStart)
	if !ok {
		return 0, fmt.Errorf("failed to convert DHCP Start IP address to netaddr struct")
	}

	var count uint32
	for ip := startingIP; ip.Less(endingIP); ip = ip.Next() {
		if existingIPAddressesSet.Contains(ip) {
			// IP address currently in use
			continue
		}
		count++
	}

	return count, nil
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
