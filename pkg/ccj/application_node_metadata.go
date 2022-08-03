package ccj

import (
	"fmt"

	"github.hpe.com/sjostrand/topology-tool/pkg/configs"
)

func BuildApplicationNodeMetadata(paddle Paddle, existingMetadata configs.ApplicationNodeMetadataMap) (configs.ApplicationNodeMetadataMap, error) {
	metadata := configs.ApplicationNodeMetadataMap{}

	for _, topologyNode := range paddle.Topology {
		if topologyNode.Type != "server" {
			continue
		}

		extraProperties, err := BuildNodeExtraProperties(topologyNode)
		if err != nil {
			return nil, fmt.Errorf("unable to build node extra properties: %w", err)
		}

		if extraProperties.Role != "Application" {
			continue
		}

		xname, err := BuildNodeXname(topologyNode, paddle, extraProperties)
		if err != nil {
			return nil, fmt.Errorf("unable to build node xname: %w", err)
		}

		// Found an application node!
		if seedMetadata, exists := existingMetadata[xname.String()]; exists {
			// This is an already existing application node
			metadata[xname.String()] = seedMetadata
		} else {
			// This is a new application node, need to have some information filled in!
			metadata[xname.String()] = configs.ApplicationNodeMetadata{
				SubRole: "~~FIXME~~",
				Aliases: []string{"~~FIXME~~"},
			}
		}

	}
	return metadata, nil
}
