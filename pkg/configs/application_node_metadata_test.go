package configs

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ApplicationNodeMetadataTestSuite struct {
	suite.Suite
}

func (suite *ApplicationNodeMetadataTestSuite) TestAllAliases() {
	applicationNodeMetadata := ApplicationNodeMetadataMap{
		"x3000c0s15b0n0": ApplicationNodeMetadata{
			SubRole: "UAN",
			Aliases: []string{"uan01"},
		},
		"x3000c0s16b0n0": ApplicationNodeMetadata{
			SubRole: "UAN",
			Aliases: []string{"uan02"},
		},
		"x3001c0s16b0n0": ApplicationNodeMetadata{
			SubRole: "UAN",
			Aliases: []string{"uan03"},
		},
	}

	expectedAliases := map[string][]string{
		"uan01": []string{"x3000c0s15b0n0"},
		"uan02": []string{"x3000c0s16b0n0"},
		"uan03": []string{"x3001c0s16b0n0"},
	}

	aliases := applicationNodeMetadata.AllAliases()
	suite.Equal(expectedAliases, aliases)
}

func (suite *ApplicationNodeMetadataTestSuite) TestAllAliases_DuplicateAliases() {
	applicationNodeMetadata := ApplicationNodeMetadataMap{
		"x3000c0s15b0n0": ApplicationNodeMetadata{
			SubRole: "UAN",
			Aliases: []string{"uan01"},
		},
		"x3000c0s16b0n0": ApplicationNodeMetadata{
			SubRole: "UAN",
			Aliases: []string{"uan01"},
		},
		"x3001c0s16b0n0": ApplicationNodeMetadata{
			SubRole: "UAN",
			Aliases: []string{"uan03"},
		},
	}

	expectedAliases := map[string][]string{
		"uan01": []string{"x3000c0s15b0n0", "x3000c0s16b0n0"},
		"uan03": []string{"x3001c0s16b0n0"},
	}

	aliases := applicationNodeMetadata.AllAliases()
	suite.Equal(expectedAliases, aliases)
}

func TestApplicationNodeMetadataTestSuite(t *testing.T) {
	suite.Run(t, new(ApplicationNodeMetadataTestSuite))
}
