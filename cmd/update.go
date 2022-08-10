/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"syscall"

	"github.com/Cray-HPE/hms-bss/pkg/bssTypes"
	dns_dhcp "github.com/Cray-HPE/hms-dns-dhcp/pkg"
	sls_client "github.com/Cray-HPE/hms-sls/pkg/sls-client"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.hpe.com/sjostrand/topology-tool/internal/engine"
	"github.hpe.com/sjostrand/topology-tool/pkg/bss"
	"github.hpe.com/sjostrand/topology-tool/pkg/ccj"
	"github.hpe.com/sjostrand/topology-tool/pkg/configs"
	"github.hpe.com/sjostrand/topology-tool/pkg/sls"
	"gopkg.in/yaml.v2"
)

// updateCmd represents the update command
var updateCmd = &cobra.Command{
	Use:   "update [CCJ_FILE]",
	Args:  cobra.ExactArgs(1),
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Initialize the global viper
		v := viper.GetViper()
		v.BindPFlags(cmd.Flags())

		// Setup Context
		ctx := setupContext()

		// TODO deal with getting an API token

		// Setup HTTP client
		httpClient := retryablehttp.NewClient()

		// Setup SLS client
		currentSLSStateLocation := v.GetString("sls-url")
		slsClient := sls_client.NewSLSClient(currentSLSStateLocation, httpClient.StandardClient(), "")

		// Setup BSS client
		bssURL := v.GetString("bss-url")
		var bssClient *bss.BSSClient
		if bssURL != "" {
			log.Printf("Using BSS at %s\n", bssURL)

			bssClient = bss.NewBSSClient(bssURL, httpClient.StandardClient(), "todo_token")
		} else {
			log.Println("Connection to BSS disabled")
		}

		// Setup HSM client
		// TODO Expand hms-dns-dhcp to perform searches of IPs and MACs
		hsmURL := v.GetString("hsm-url")

		dns_dhcp.NewDHCPDNSHelper(hsmURL, httpClient)

		//
		// Parse input files
		//
		ccjFile := args[0]
		log.Printf("Using CCJ file at %s\n", ccjFile)

		// Read in the paddle file
		paddleRaw, err := ioutil.ReadFile(ccjFile)
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

		// Read in application_node_config.yaml
		// TODO the prefixes list is not being used, as we are assuming all unknown nodes are application
		applicationNodeMetadataFile := v.GetString("application-node-metadata")
		var applicationNodeMetadata configs.ApplicationNodeMetadataMap
		if applicationNodeMetadataFile == "" {
			log.Printf("No application node metadata file provided.\n")
		} else {
			log.Printf("Using application node metadata file at %s\n", applicationNodeMetadataFile)
			applicationNodeMetadataRaw, err := ioutil.ReadFile(applicationNodeMetadataFile)
			if err != nil {
				log.Fatal("Error:", err)
			}

			if err := yaml.Unmarshal(applicationNodeMetadataRaw, &applicationNodeMetadata); err != nil {
				log.Fatal("Error:", err)
			}
		}

		//
		// Retrieve current state from the system
		//
		log.Printf("Retrieving current SLS state from %s\n", currentSLSStateLocation)

		currentSLSState, err := slsClient.GetDumpState(ctx)
		if err != nil {
			log.Fatal("Error:", err)
		}

		// TOOD Ideally we could add the initial set of hardware to SLS, if the systems networking information
		// is known.
		if len(currentSLSState.Networks) == 0 {
			log.Fatal("Refusing to continue as the current SLS state does not contain networking information")
		}

		// Build up the application node metadata for the current state of the system
		currentApplicationNodeMetadata, err := sls.BuildApplicationNodeMetadata(currentSLSState.Hardware)
		if err != nil {
			log.Fatal("Error:", err)
		}

		foundDuplicates := false
		for alias, xnames := range currentApplicationNodeMetadata.AllAliases() {
			if len(xnames) > 1 {
				log.Printf("Alias %s is used by multiple application nodes: %s\n", alias, strings.Join(xnames, ","))
			}
		}
		if foundDuplicates {
			log.Fatal("The current SLS state contains application nodes that share the same alias. Please reconcile before continuing.")
		}

		if applicationNodeMetadataFile == "" {
			// Build up the application metadata config for the expected state of the system if no file was provided.
			applicationNodeMetadata, err = ccj.BuildApplicationNodeMetadata(paddle, currentApplicationNodeMetadata)
			if err != nil {
				log.Fatal("Error:", err)
			}
		}

		// At this point we can detect if any application nodes are missing required data
		// TODO Should we verify all SubRoles are valid against HSM?
		foundFixMes := false
		for xname, metadata := range applicationNodeMetadata {
			if metadata.SubRole == "~~FIXME~~" {
				log.Printf("Application node %s has SubRole of ~~FIXME~~\n", xname)
				foundFixMes = true
			}

			for _, alias := range metadata.Aliases {
				if alias == "~~FIXME~~" {
					log.Printf("Application node %s has Alias of ~~FIXME~~\n", xname)
					foundFixMes = true
				}
			}
		}
		if foundFixMes {
			// TODO Rephrase
			// TODO the summary wording if a an application node metadata file is needed could be improved
			log.Println()
			log.Println("New Application nodes are being added to the system which requires additional metadata to be provided.")
			log.Println("Please fill in all of the ~~FIXME~~ values in the application node metadata file.")
			log.Println()

			if applicationNodeMetadataFile == "" {
				// Since no application node metadata file was provided and required information is not present,
				// write it out so the missing information can be filled in.
				applicationNodeMetadataFile = "application_node_metadata.yaml"

				// Check to see if the file exists
				if _, err := os.Stat(applicationNodeMetadataFile); err == nil {
					log.Printf("Add --application-node-metadata=%s to the command line arguments and try again.\n", applicationNodeMetadataFile)
					log.Fatalf("Error %s already exists in the current directory. Refusing to overwrite!\n", applicationNodeMetadataFile)
				}

				// Write it out!
				log.Printf("Application node metadata file is now available at: %s\n", applicationNodeMetadataFile)
				log.Printf("Add --application-node-metadata=%s to the command line arguments and try again.\n", applicationNodeMetadataFile)
				applicationNodeMetadataRaw, err := yaml.Marshal(applicationNodeMetadata)
				if err != nil {
					log.Fatal("Error:", err)
				}

				err = ioutil.WriteFile(applicationNodeMetadataFile, applicationNodeMetadataRaw, 0600)
				if err != nil {
					log.Fatal("Error:", err)
				}
			}

			os.Exit(1)
		}

		foundDuplicates = false
		for alias, xnames := range applicationNodeMetadata.AllAliases() {
			if len(xnames) > 1 {
				log.Printf("Alias %s is used by multiple application nodes: %s\n", alias, strings.Join(xnames, ","))
				foundDuplicates = true
			}
		}
		if foundDuplicates {
			log.Println("The proposed SLS state contains application nodes that share the same alias.")
			log.Fatalf("Error found duplicate application node aliases. Verify all application nodes being added to the system have unique aliases defined in %s\n", applicationNodeMetadataFile)
		}

		// Retrieve BSS data
		managementNCNs, err := sls.FindManagementNCNs(currentSLSState.Hardware)
		if err != nil {
			log.Fatal("Error:", err)
		}

		log.Println("Retrieving Global boot parameters from BSS")
		bssGlobalBootParameters, err := bssClient.GetBSSBootparametersByName("Global")
		if err != nil {
			log.Fatal("Error:", err)
		}

		managementNCNBootParams := map[string]*bssTypes.BootParams{}
		for _, managementNCN := range managementNCNs {
			log.Printf("Retrieving boot parameters for %s from BSS\n", managementNCN.Xname)
			bootParams, err := bssClient.GetBSSBootparametersByName(managementNCN.Xname)
			if err != nil {
				log.Fatal("Error:", err)
			}

			managementNCNBootParams[managementNCN.Xname] = bootParams
		}

		//
		// Determine topology changes
		//
		topologyEngine := engine.TopologyEngine{
			Input: engine.EngineInput{
				Paddle:                  paddle,
				ApplicationNodeMetadata: applicationNodeMetadata,
				CurrentSLSState:         currentSLSState,
			},
		}

		topologyChanges, err := topologyEngine.DetermineChanges()
		if err != nil {
			log.Fatal("Error:", err)
		}

		// Merge Topology Changes into the current SLS state
		for name, network := range topologyChanges.ModifiedNetworks {
			currentSLSState.Networks[name] = network
		}
		for _, hardware := range topologyChanges.HardwareAdded {
			currentSLSState.Hardware[hardware.Xname] = hardware
		}

		{
			//
			// Debug stuff
			//
			topologyChangesRaw, err := json.MarshalIndent(topologyChanges, "", "  ")
			if err != nil {
				log.Fatal("Error:", err)
			}

			ioutil.WriteFile("topology_changes.json", topologyChangesRaw, 0600)
		}

		//
		// Check to see if any newly allocated IPs are currently in use
		//
		for _, event := range topologyChanges.IPReservationsAdded {
			_ = event

			// TODO check HSM EthernetInterfaces to see if this IP is in use.
			// ip := event.IPReservation.IPAddress
		}

		//
		// Determine changes requires to downstream services from SLS. Like HSM and BSS
		//

		// TODO For right now lets just always push the host records, unless the reflect.DeepEqual
		// says they are equal.
		// Because the logic to compare the expected BSS host records with the current ones
		// is kind of hard. If anything is different just recalculate it.
		// This should be harmless just the order of records will shift around.

		// Recalculate the systems host recorded
		modifiedGlobalBootParameters := false
		expectedGlobalHostRecords := bss.GetBSSGlobalHostRecords(managementNCNs, sls.Networks(currentSLSState))

		var currentGlobalHostRecords bss.HostRecords
		if err := mapstructure.Decode(bssGlobalBootParameters.CloudInit.MetaData["host_records"], &currentGlobalHostRecords); err != nil {
			log.Fatal("Error:", err)
		}

		if !reflect.DeepEqual(currentGlobalHostRecords, expectedGlobalHostRecords) {
			log.Println("Host records in BSS Global boot parameters are out of date")
			bssGlobalBootParameters.CloudInit.MetaData["host_records"] = expectedGlobalHostRecords
			modifiedGlobalBootParameters = true
		}

		// Recalculate cabinet routes
		// TODO NOTE this is the list of the managementNCNs before the topology of SLS changed.
		modifiedManagementNCNBootParams := map[string]bool{}

		for _, managementNCN := range managementNCNs {

			// The following was stolen from CSI
			extraNets := []string{}
			var foundCAN = false
			var foundCHN = false

			for _, net := range sls.Networks(currentSLSState) {
				if strings.ToLower(net.Name) == "can" {
					extraNets = append(extraNets, "can")
					foundCAN = true
				}
				if strings.ToLower(net.Name) == "chn" {
					foundCHN = true
				}
			}
			if !foundCAN && !foundCHN {
				log.Fatal("Error no CAN or CHN network defined in SLS networks")
			}

			// IPAM
			ipamNetworks := bss.GetIPAMForNCN(managementNCN, sls.Networks(currentSLSState), extraNets...)
			expectedWriteFiles := bss.GetWriteFiles(sls.Networks(currentSLSState), ipamNetworks)

			var currentWriteFiles []bss.WriteFile
			if err := mapstructure.Decode(managementNCNBootParams[managementNCN.Xname].CloudInit.UserData["write_files"], &currentWriteFiles); err != nil {
				panic(err)
			}

			// TODO For right now lets just always push the writefiles, unless the reflect.DeepEqual
			// says they are equal.
			// This should be harmless, the cabinet routes may be in a different order. This is due to cabinet routes do not overlap with each other.
			if !reflect.DeepEqual(expectedWriteFiles, currentWriteFiles) {
				log.Printf("Cabinet routes for %s in BSS Global boot parameters are out of date\n", managementNCN.Xname)
				managementNCNBootParams[managementNCN.Xname].CloudInit.UserData["write_files"] = expectedWriteFiles
				modifiedManagementNCNBootParams[managementNCN.Xname] = true
			}

		}

		//
		// Perform changes to SLS/HSM/BSS on the system to reflect the updated state.
		//

		// Add new hardware
		if len(topologyChanges.HardwareAdded) == 0 {
			log.Println("No hardware added")
		} else {
			log.Printf("Adding new hardware to SLS (count %d)\n", len(topologyChanges.HardwareAdded))
			for _, hardware := range topologyChanges.HardwareAdded {
				if slsClient != nil {
					slsClient.PutHardware(ctx, hardware)
				}
			}
		}

		// Update modified networks
		if len(topologyChanges.ModifiedNetworks) == 0 {
			log.Println("No SLS network changes required")
		} else {
			log.Printf("Updating modified networks in SLS (count %d)\n", len(topologyChanges.ModifiedNetworks))
			for _, modifiedNetwork := range topologyChanges.ModifiedNetworks {
				if slsClient != nil {
					err := slsClient.PutNetwork(ctx, modifiedNetwork)
					if err != nil {
						log.Fatal("Error:", err)
					}
				}
			}
		}

		// Update BSS Global Bootparams
		if !modifiedGlobalBootParameters {
			log.Println("No BSS Global boot parameters changes required")
		} else {
			log.Println("Updating BSS Global boot parameters")
			_, err := bssClient.UploadEntryToBSS(*bssGlobalBootParameters, http.MethodPut)
			if err != nil {
				log.Fatal("Error:", err)
			}
		}

		// Update per NCN BSS Boot parameters
		for _, managementNCN := range managementNCNs {
			xname := managementNCN.Xname

			if !modifiedManagementNCNBootParams[xname] {
				log.Printf("No changes to BSS boot parameters for %s\n", xname)
				continue
			}
			log.Printf("Updating BSS boot parameters for %s\n", xname)
			_, err := bssClient.UploadEntryToBSS(*managementNCNBootParams[xname], http.MethodPut)
			if err != nil {
				log.Fatal("Error:", err)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// updateCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// updateCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	updateCmd.Flags().String("sls-url", "http://localhost:8376", "URL to SLS")
	updateCmd.Flags().String("bss-url", "http://localhost:27778", "URL to BSS")
	updateCmd.Flags().String("hsm-url", "http://localhost:27779", "URL to HSM")

	updateCmd.Flags().String("application-node-metadata", "", "YAML to control Application node identification during the SLS State generation. Only required if application nodes are being added to the system")
}

func setupContext() context.Context {
	var cancel context.CancelFunc
	ctx, cancel := context.WithCancel(context.Background())

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-c

		// Cancel the context to cancel any in progress HTTP requests.
		cancel()
	}()

	return ctx
}
