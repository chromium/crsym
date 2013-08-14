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

package breakpad

import (
	"path"
	"strings"
	"testing"

	"github.com/chromium/crsym/testutils"
)

const (
	kRemotingFile     = "remoting_host_DDD03DFA61DBE06E5910763112FB57BA0.breakpad"
	kChromeHelperFile = "google_chrome_helper_605A7422B1101728E9B1EAAA1F1E52480.breakpad"
	kAppKitFile       = "AppKit_A353465ECFC9CB75949D786F6F7732F60.breakpad"
	kBreakpadTestFile = "omap_stretched_filled.sym" // From https://code.google.com/p/google-breakpad/source/browse/trunk/src/tools/windows/dump_syms/testdata/omap_stretched_filled.sym?spec=svn1167&r=1167
	kChromeFramework  = "google_chrome_framework_4FD3F4B39DD03B76824ED233842F6A300.breakpad"
)

func getTable(file string) (*breakpadFile, error) {
	data, err := testutils.ReadSourceFile(path.Join("breakpad/testdata", file))
	if err != nil {
		return nil, err
	}

	table, err := NewBreakpadSymbolTable(string(data))
	if err != nil {
		return nil, err
	}

	bf, _ := table.(*breakpadFile)
	return bf, nil
}

func TestParseRemoting(t *testing.T) {
	bf, err := getTable(kRemotingFile)
	if err != nil {
		t.Fatal(err)
	}

	meta := []struct {
		field    string
		actual   string
		expected string
	}{
		{"osname", bf.osname, "mac"},
		{"arch", bf.arch, "x86"},
		{"ident", bf.ident, "DDD03DFA61DBE06E5910763112FB57BA0"},
		{"module", bf.module, "remoting_host_plugin"},
	}
	for _, r := range meta {
		if r.actual != r.expected {
			t.Errorf("%s invalid, expected '%s', got '%s'", r.field, r.expected, r.actual)
		}
	}

	// Use `grep '^FUNC ' file.breakpad | wc -l` to get expectations.
	lens := []struct {
		field    string
		actual   int
		expected int
	}{
		{"files", len(bf.files), 1472},
		{"funcs", len(bf.funcs), 15100},
		{"publics", len(bf.publics), 19991},
	}
	for _, r := range lens {
		if r.actual != r.expected {
			t.Errorf("number of %s does not meet expectations, expected %d, got %d", r.field, r.expected, r.actual)
		}
	}

	files := []struct {
		index    int64
		expected string
	}{
		{0, "../../base/logging.h"},                                          // First record.
		{42, "../base/memory/scoped_ptr.h"},                                  // Answer to life.
		{314, "/b/build/slave/chrome-official-mac/build/src/base/base64.cc"}, // Pi.
		{1471, "src/google/protobuf/wire_format_lite_inl.h"},                 // Last record.
	}
	for _, r := range files {
		actual := bf.files[r.index]
		if actual != r.expected {
			t.Errorf("file at index %d invalid, expected '%s', got '%s'", r.index, r.expected, actual)
		}
	}
}

func TestSymbolForAddressRemoting(t *testing.T) {
	bf, err := getTable(kRemotingFile)
	if err != nil {
		t.Fatal(err)
	}

	results := []struct {
		address uint64
		symbol  string
		file    string
		line    int
	}{
		{0x2c60, "remoting::::DaemonControllerMac::DoUpdateConfig", "/b/build/slave/chrome-official-mac/build/src/remoting/host/plugin/daemon_controller_mac.cc", 231},
		{0x2d83, "remoting::::DaemonControllerMac::DoUpdateConfig", "/Developer/SDKs/MacOSX10.5.sdk/usr/include/c++/4.2.1/bits/basic_string.h", 226},
		{0x181420, "non-virtual thunk to net::HostResolverImpl::~HostResolverImpl()", "", 0},
		{0xf5a89c, "Singleton<base::debug::TraceLog, StaticMemorySingletonTraits<base::debug::TraceLog>, base::debug::TraceLog>::instance_", "", 0},
	}

	for _, r := range results {
		actual := bf.SymbolForAddress(r.address)
		if actual == nil {
			t.Errorf("address %x is nil and should not be", r.address)
			continue
		}
		if actual.Function != r.symbol {
			t.Errorf("address %x should be '%s', got '%s'", r.address, r.symbol, actual.Function)
		}
		if actual.File != r.file {
			t.Errorf("file for address %x should be '%s', got '%s'", r.address, r.file, actual.File)
		}
		if actual.Line != r.line {
			t.Errorf("line for address %x should be %d, got %d", r.address, r.line, actual.Line)
		}
	}
}

func TestSpacesInStrings(t *testing.T) {
	data := `MODULE mac x86 73C5EC60C2EA7343C2495AB71C16B32B0 A Module With Spaces
FILE 0 /Volumes/Source Path/project/main.cc
FUNC 1f4a9 20 0 Allays::IBF(int, int*) const
1f4a9 4 55 0
PUBLIC abc123 0 CreateDelegate(int, void**)
`

	table, err := NewBreakpadSymbolTable(data)
	if err != nil {
		t.Fatal(err)
	}

	bf, _ := table.(*breakpadFile)

	actual := bf.module
	expected := "A Module With Spaces"
	if actual != expected {
		t.Errorf("module name failed, expected '%s', got '%s'", expected, actual)
	}

	actual = bf.files[0]
	expected = "/Volumes/Source Path/project/main.cc"
	if actual != expected {
		t.Errorf("file path failed, expected '%s', got '%s'", expected, actual)
	}

	actual = bf.funcs[0].name
	expected = "Allays::IBF(int, int*) const"
	if actual != expected {
		t.Errorf("func failed, expected '%s', got '%s'", expected, actual)
	}

	actual = bf.publics[0].name
	expected = "CreateDelegate(int, void**)"
	if actual != expected {
		t.Errorf("public failed, expected '%s', got '%s'", expected, actual)
	}
}

func TestPublicModuleAddressing(t *testing.T) {
	table, err := getTable(kAppKitFile)
	if err != nil {
		t.Fatal(err)
	}

	const kBaseAddress = 0x95d15000
	expected := map[uint64]string{
		0x95d9a73d:              "-[NSCarbonMenuImpl _carbonTargetItemEvent:handlerCallRef:]",
		0x95d8eed6:              "NSSLMGlobalEventHandler",
		0x96027a75:              "-[NSCarbonMenuImpl _popUpContextMenu:withEvent:forView:withFont:]",
		0x95e72b27:              "-[NSWindow sendEvent:]",
		0x95d8b60a:              "-[NSApplication sendEvent:]",
		0x95d1f252:              "-[NSApplication run]",
		0x1e1c + kBaseAddress:   "+[NSBinder load]",                                // First symbol by address.
		0x94f118 + kBaseAddress: ".objc_class_name_NSAccessibilityRemoteUIElement", // Last symbol.
	}

	for addr, function := range expected {
		symbol := table.SymbolForAddress(addr - kBaseAddress)
		if symbol == nil {
			t.Errorf("Could not find symbol for 0x%x", addr)
			continue
		}

		if symbol.Function != function {
			t.Errorf("Symbol for address 0x%x should be '%s', got '%s'", addr, function, symbol.Function)
		}
	}
}

func TestIgnoredStackLines(t *testing.T) {
	table, err := getTable(kChromeHelperFile)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[uint64]string{
		0xf40:  "main",
		0x1034: "NXArgv",
		0x103c: "dyld__mach_header",
	}

	for addr, function := range expected {
		symbol := table.SymbolForAddress(addr)
		if symbol == nil {
			t.Errorf("Could not find symbol for 0x%x", addr)
			continue
		}

		if symbol.Function != function {
			t.Errorf("Symbol for address 0x%x should be '%s', got '%s'", addr, function, symbol.Function)
		}
	}
}

func TestParseWindowsPDB(t *testing.T) {
	table, err := getTable(kBreakpadTestFile)
	if err != nil {
		t.Fatal(err)
	}

	symbol := table.SymbolForAddress(0x115e)
	if symbol == nil {
		t.Fatal("Could not look up symbol")
	}

	prefix := "type_info::~type_info("
	if !strings.HasPrefix(symbol.Function, prefix) {
		t.Errorf("Symbol should be %q, got %q", prefix, symbol.Function)
	}
}

func TestReadingMissingPublics(t *testing.T) {
	table, err := getTable(kChromeFramework)
	if err != nil {
		t.Fatal(err)
	}

	symbol := table.SymbolForAddress(0x7919)
	if symbol == nil {
		t.Errorf("Failed to look up symbol")
	}

	symbol = table.SymbolForAddress(0xabc)
	if symbol != nil {
		t.Errorf("Found symbol for bad address")
	}
}
