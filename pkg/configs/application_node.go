package configs

// The Key is xname as that is the only thing that can link the CCJ and SLS.
type ApplicationNodeMetadataMap map[string]ApplicationNodeMetadata

type ApplicationNodeMetadata struct {
	SubRole string
	Aliases []string
}
