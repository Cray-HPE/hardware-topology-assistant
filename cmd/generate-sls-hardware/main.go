package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.hpe.com/sjostrand/topology-tool/pkg/ccj"
	"github.hpe.com/sjostrand/topology-tool/pkg/configs"
	"gopkg.in/yaml.v2"
)

// TODO liquid-cooled nodes are missing
// TODO management NCNs are missing NIDs
// TODO network hardware objects are missing IPs (which is a hold over)

func main() {
	if len(os.Args) != 3 {
		panic("Incorrect number of CLI args provided")
	}

	// Read in the paddle file
	paddleRaw, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}

	var paddle ccj.Paddle
	if err := json.Unmarshal(paddleRaw, &paddle); err != nil {
		panic(err)
	}

	// TODO Verify Paddle
	// - Check CANU Version?
	// - Check Architecture against list of supported

	supportedArchitectures := map[string]bool{
		"network_v2_tds": true,
		"network_v2":     true,
		"network_v1":     true,
	}
	if !supportedArchitectures[paddle.Architecture] {
		err := fmt.Errorf("unsupported paddle architecture (%v)", paddle.Architecture)
		panic(err)
	}

	// Determine the cabinet in cabinet lookup
	cabinetLookup, err := ccj.DetermineCabinetLookup(paddle)
	if err != nil {
		panic(err)
	}

	// TODO remove for testing...
	{
		jsonRaw, err := json.MarshalIndent(cabinetLookup, "", "  ")
		if err != nil {
			panic(err)
		}

		fmt.Println("DEBUG: Cabinet lookup")
		fmt.Println(string(jsonRaw))
	}

	// Read in application_node_metadata.yaml
	applicationNodeMetadataRaw, err := ioutil.ReadFile(os.Args[2])
	if err != nil {
		panic(err)
	}

	var applicationNodeMetadata configs.ApplicationNodeMetadataMap
	if err := yaml.Unmarshal(applicationNodeMetadataRaw, &applicationNodeMetadata); err != nil {
		panic(err)
	}

	slsState, err := ccj.BuildExpectedHardwareState(paddle, cabinetLookup, applicationNodeMetadata, nil)
	if err != nil {
		panic(err)
	}

	// Write out the SLS State file
	slsSateRaw, err := json.MarshalIndent(slsState, "", "  ")
	if err != nil {
		panic(err)
	}

	err = ioutil.WriteFile("sls_state.json", slsSateRaw, 0600)
	if err != nil {
		panic(err)
	}
}
