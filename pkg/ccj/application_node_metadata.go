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

package ccj

import (
	"fmt"

	"github.com/Cray-HPE/hardware-topology-assistant/pkg/configs"
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
