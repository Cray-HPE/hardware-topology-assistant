package main

import (
	"fmt"

	"github.com/Cray-HPE/hms-xname/xnames"
)

func GetBogusNID(xname string) uint64 {
	var cab, chassis, slot, controller, node uint
	fmt.Sscanf(xname, "x%dc%ds%db%dn%d", &cab, &chassis, &slot, &controller, &node)
	nid := ((cab + 1) * 16384) + (chassis * 2048) + (slot * 32) + (controller * 4) + node
	return uint64(nid)
}

func main() {
	for cabinet := 1000; cabinet <= 1000; cabinet++ {
		for chassis := 0; chassis <= 0; chassis++ {
			for slot := 0; slot <= 1; slot++ {
				for bmc := 0; bmc <= 1; bmc++ {
					for node := 0; node <= 1; node++ {
						xname := xnames.Node{
							Cabinet:       cabinet,
							Chassis:       chassis,
							ComputeModule: slot,
							NodeBMC:       bmc,
							Node:          node,
						}

						fmt.Println(xname, GetBogusNID(xname.String()))
						// fmt.Println(GetBogusNID(xname.String()))
					}
				}
			}
		}
	}
}
