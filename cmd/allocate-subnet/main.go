package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.hpe.com/sjostrand/topology-tool/pkg/ipam"
	"github.hpe.com/sjostrand/topology-tool/pkg/sls"
)

func main() {

	slsStateRaw, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}

	var slsState sls_common.SLSState
	if err := json.Unmarshal(slsStateRaw, &slsState); err != nil {
		panic(err)
	}

	slsNetwork, ok := slsState.Networks["NMN_MTN"]
	if !ok {
		panic("network does not exist")
	}

	// Map this network to a usable structure.
	var networkExtraProperties sls_common.NetworkExtraProperties
	err = sls.DecodeNetworkExtraProperties(slsNetwork.ExtraPropertiesRaw, &networkExtraProperties)
	if err != nil {
		log.Fatalf("Failed to decode raw network extra properties to correct structure: %s", err)
	}

	subnet, err := ipam.FindNextAvailableSubnet(networkExtraProperties)
	if err != nil {
		panic(err)
	}

	fmt.Println(subnet)

}
