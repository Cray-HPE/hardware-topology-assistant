package configs

import "sort"

// The Key is xname as that is the only thing that can link the CCJ and SLS.
type ApplicationNodeMetadataMap map[string]ApplicationNodeMetadata

type ApplicationNodeMetadata struct {
	SubRole string
	Aliases []string
}

func (m ApplicationNodeMetadataMap) AllAliases() map[string][]string {
	allAliases := map[string][]string{}
	for xname, metadata := range m {
		for _, alias := range metadata.Aliases {
			allAliases[alias] = append(allAliases[alias], xname)
		}
	}

	for _, nodes := range allAliases {
		sort.Strings(nodes)
	}

	return allAliases

}
