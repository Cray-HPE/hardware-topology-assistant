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
