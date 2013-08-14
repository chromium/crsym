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

package frontend

import (
	"fmt"
	"path"
	"testing"

	"github.com/chromium/crsym/breakpad"
	"github.com/chromium/crsym/testutils"
)

func TestBinaryImage(t *testing.T) {
	image := binaryImage{
		baseAddress: 0x101,
		name:        "com.google.Chrome",
		ident:       "D54FE0E8-24AB-4893-859C-F26797170CC2",
		path:        "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	}

	expected := "D54FE0E824AB4893859CF26797170CC20"
	actual := image.breakpadUUID()
	if expected != actual {
		t.Errorf("breakpadUUID should be '%s', got '%s'", expected, actual)
	}

	expected = "Google Chrome"
	actual = image.breakpadName()
	if expected != actual {
		t.Errorf("breakpad name should be '%s', got '%s'", expected, actual)
	}
}

func TestParseBinaryImages(t *testing.T) {
	report := `Report Version: 6
Binary Images:
0x491e5000 - 0x491e5ff7 +com.google.Chrome 20.0.1132.42 (1132.42) <cf4d75d8804d775084d363a5cbbf7702> /Applications/Google Chrome.app/Contents/MacOS/Google Chrome
0x520ce000 - 0x520ceff7 +com.google.Chrome.canary 17.0.959.0 (959.0) <8BC87704-1B47-6F0C-70DE-17F7A99A1E45> /Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary`

	parser := new(AppleInputParser)
	err := parser.ParseInput(report)
	if err != nil {
		t.Fatalf("Unexpected error parsing input: %v", err)
	}

	actual, ok := parser.modules["com.google.Chrome"]
	if !ok {
		t.Errorf("Could not find module com.google.Chrome")
	} else {
		if actual.baseAddress != 0x491e5000 {
			t.Errorf("Unexpected base address for %#v", actual)
		}
		expected := "CF4D75D8804D775084D363A5CBBF77020"
		if actual.breakpadUUID() != expected {
			t.Errorf("Wrong breakpadUUID, expected '%s', got '%s'", expected, actual.breakpadUUID())
		}
	}

	actual, ok = parser.modules["com.google.Chrome.canary"]
	if !ok {
		t.Errorf("Could not find module com.google.Chrome.canary")
	} else {
		if actual.baseAddress != 0x520ce000 {
			t.Errorf("Unexpected base address for %#v", actual)
		}
		expected := "8BC877041B476F0C70DE17F7A99A1E450"
		if actual.breakpadUUID() != expected {
			t.Errorf("Wrong breakpadUUID, expected '%s', got '%s'", expected, actual.breakpadUUID())
		}
	}
}

func TestReportVersion(t *testing.T) {
	expectations := map[string]bool{
		"9":   true,
		"0x8": false,
		"foo": false,
		"10":  true,
	}

	for version, allowed := range expectations {
		p := new(AppleInputParser)
		err := p.ParseInput(fmt.Sprintf("Report Version:     %s", version))
		if (err != nil && allowed) || (err == nil && !allowed) {
			t.Errorf("Report Version '%s' should be allowed: %t. Got error: %v", version, allowed, err)
		}
	}
}

func TestParseAppleInput(t *testing.T) {
	expected := []struct {
		filename      string
		reportVersion int
		images        []binaryImage
	}{
		{
			"crash_10.7_v9.crash",
			9,
			[]binaryImage{
				binaryImage{
					0x4c000,
					"com.google.Chrome.canary",
					"26A6C8D5-C994-73CA-195E-55656E111C97",
					"Google Chrome Canary",
				},
				binaryImage{
					0x51000,
					"com.google.Chrome.framework",
					"18D7EF91-5100-665A-BE61-EC3140EADD1A",
					"Google Chrome Framework",
				},
			},
		},
	}

	for _, e := range expected {
		data, err := testutils.ReadSourceFile(testdata(e.filename))
		if err != nil {
			t.Error(err)
			continue
		}

		parser := new(AppleInputParser)
		err = parser.ParseInput(string(data))
		if err != nil {
			t.Error(err)
		}

		if parser.reportVersion != e.reportVersion {
			t.Errorf("Report version mismatch for %s, expected %d, got %d", e.filename, e.reportVersion, parser.reportVersion)
		}

		for _, image := range e.images {
			actual, ok := parser.modules[image.name]
			if !ok {
				t.Errorf("Could not find module %s", image.name)
				continue
			}

			if actual.baseAddress != image.baseAddress {
				t.Errorf("Base address for %s in %s wrong, expected 0x%x, got 0x%x", image.name, e.filename, image.baseAddress, actual.baseAddress)
			}
			if actual.ident != image.ident {
				t.Errorf("UUID for %s in %s is wrong, expected '%s', got '%s'", image.name, e.filename, image.ident, actual.ident)
			}
			lastComponent := path.Base(actual.path)
			if image.path != lastComponent {
				t.Errorf("Last path component for %s in %s is wrong, expected '%s', got '%s'", image.name, e.filename, image.path, lastComponent)
			}
		}
	}
}

func TestSymbolizeApple(t *testing.T) {
	// and it will put the .actual file on your local machine, rather than on Forge.
	// You can then copy the file to the testdata/ directory.
	files := []string{
		"crash_10.8_v10.crash",
		"crash_10.8_v10_2.crash",
		"crash_10.7_v9.crash",
		"crash_10.6_v6.crash",
		"hang_10.7_v7.crash",
		"hang_10.8_v7.crash",
	}

	for _, input := range files {
		inputData, err := testutils.ReadSourceFile(testdata(input))
		if err != nil {
			t.Errorf("Failed to read file: %v", err)
			continue
		}

		tables := []breakpad.SymbolTable{
			&testTable{name: "Google Chrome Framework", symbol: "Framework"},
			&testTable{name: "Google Chrome Canary", symbol: "Chrome"},
		}

		parser := new(AppleInputParser)
		err = parser.ParseInput(string(inputData))
		if err != nil {
			t.Errorf("%s: %s", input, err)
			continue
		}

		expectedFileName := testdata(input + ".expected")
		outputData, err := testutils.ReadSourceFile(expectedFileName)
		if err != nil {
			t.Errorf("%s.expected: %s", input, err)
		}

		actual := parser.Symbolize(tables)

		if actual != string(outputData) {
			actualFileName, actualFile, err := testutils.CreateTempFile(input + ".actual")
			if err != nil {
				t.Errorf("Could not create actual file output: %v", err)
				continue
			}

			fmt.Fprint(actualFile, actual)
			actualFile.Close()

			t.Errorf("Input data for %s does not symbolize to expected output", input)
			line := 1
			for i := 0; i < len(actual) && i < len(outputData); i++ {
				if actual[i] == '\n' {
					line++
				}
				if actual[i] != outputData[i] {
					t.Errorf("  First mismatch at byte %d (actual output line %d) %#x != %#x",
						i, line, actual[i], outputData[i])
					t.Logf("    Around [ actual ] %q", actual[max(0, i-30):min(i+30, len(actual))])
					t.Logf("    Around [expected] %q", string(outputData[max(0, i-30):min(i+30, len(outputData))]))
					break
				}
			}

			t.Errorf("  Expected output: %s", testutils.GetSourceFilePath(expectedFileName))
			t.Errorf("  Actual output: %s", actualFileName)
		}
	}
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
