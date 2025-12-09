// Copyright IBM Corp. 2017, 2025
// SPDX-License-Identifier: MPL-2.0

package version

import "fmt"

const Version = "0.2.0"

var (
	Name      string
	GitCommit string

	HumanVersion = fmt.Sprintf("%s v%s (%s)", Name, Version, GitCommit)
)
