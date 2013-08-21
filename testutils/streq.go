/* Copyright 2013 Google Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package testutils

import (
	"errors"
	"fmt"
	"io/ioutil"
)

// CheckStringsEqual ensures that the actual string matches the expected. If the
// strings match, returns nil. If they do not, returns an error describing the
// difference.
func CheckStringsEqual(expected, actual string) error {
	if expected == actual {
		return nil
	}

	var msg string
	line := 1
	for i := 0; i < len(actual) && i < len(expected); i++ {
		if actual[i] == '\n' {
			line++
		}
		if actual[i] != expected[i] {
			msg = fmt.Sprintf("  First mismatch at byte %d (actual output line %d) %#x != %#x",
				i, line, actual[i], expected[i])

			lower := max(0, i-30)
			msg += fmt.Sprintf("\n    Around [ actual ] %q", actual[lower:min(i+30, len(actual))])
			msg += fmt.Sprintf("\n    Around [expected] %q", expected[lower:min(i+30, len(expected))])
			break
		}
	}

	return errors.New(msg)
}

// CheckFilesEqual ensures that the contents of the expected file has the same
// string contents, according to CheckStringsEqual, as the actual file.
func CheckFilesEqual(expectedFile, actualFile string) error {
	expected, err := ioutil.ReadFile(expectedFile)
	if err != nil {
		return fmt.Errorf("CheckFilesEqual: cannot read expected file: %v", err)
	}

	actual, err := ioutil.ReadFile(actualFile)
	if err != nil {
		return fmt.Errorf("CheckFilesEqual: cannot read actual file: %v", err)
	}

	err = CheckStringsEqual(string(expected), string(actual))
	if err != nil {
		return fmt.Errorf("%v\n  Expected file: %s\n  Actual file:%s", err, expectedFile, actualFile)
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
