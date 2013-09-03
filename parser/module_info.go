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

package parser

import (
	"fmt"
	"strings"

	"github.com/chromium/crsym/breakpad"
	"github.com/chromium/crsym/context"
)

type moduleInfoParser struct {
	context          context.Context
	service          breakpad.ModuleInfoService
	product, version string
	modules          []breakpad.SupplierRequest
}

// NewModuleInfoParser creates an input parser that takes a product name and
// version, along with a backend service, and will look up all the modules for that
// tuple.
func NewModuleInfoParser(ctx context.Context, service breakpad.ModuleInfoService, product, version string) Parser {
	return &moduleInfoParser{
		context: ctx,
		service: service,
		product: product,
		version: version,
	}
}

func (p *moduleInfoParser) ParseInput(data string) (err error) {
	p.modules, err = p.service.GetModulesForProduct(p.context, p.product, p.version)
	return
}

func (p *moduleInfoParser) RequiredModules() []breakpad.SupplierRequest {
	return nil
}

func (p *moduleInfoParser) FilterModules() bool {
	return false
}

func (p *moduleInfoParser) Symbolize(tables []breakpad.SymbolTable) string {
	lines := make([]string, len(p.modules))
	for i, module := range p.modules {
		lines[i] = fmt.Sprintf("\"%s\"\t\t%s", module.ModuleName, module.Identifier)
	}
	return strings.Join(lines, "\n")
}
