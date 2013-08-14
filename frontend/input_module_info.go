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
	"strings"

	"github.com/chromium/crsym/breakpad"
)

type moduleInfoInputParser struct {
	service          breakpad.ModuleInfoService
	product, version string
	modules          []breakpad.SupplierRequest
}

// NewModuleInfoInputParser creates an input parser that takes a product name and
// version, along with a backend service, and will look up all the modules for that
// tuple.
func NewModuleInfoInputParser(service breakpad.ModuleInfoService, product, version string) InputParser {
	return &moduleInfoInputParser{
		service: service,
		product: product,
		version: version,
	}
}

func (p *moduleInfoInputParser) ParseInput(data string) (err error) {
	p.modules, err = p.service.GetModulesForProduct(p.product, p.version)
	return
}

func (p *moduleInfoInputParser) RequiredModules() []breakpad.SupplierRequest {
	return nil
}

func (p *moduleInfoInputParser) FilterModules() bool {
	return false
}

func (p *moduleInfoInputParser) Symbolize(tables []breakpad.SymbolTable) string {
	lines := make([]string, len(p.modules))
	for i, module := range p.modules {
		lines[i] = fmt.Sprintf("\"%s\"\t\t%s", module.ModuleName, module.Identifier)
	}
	return strings.Join(lines, "\n")
}
