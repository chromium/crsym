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

// Supplier is an interface that can take a SymbolRequest and furnish a SymbolTable
// in response, via a SupplierResponse.
type Supplier interface {
	// FilterAvailableModules allows Supplier to filter down a list of input modules
	// if it has apriori knowledge of which SymbolTables it can return. This
	// potentially eliminates unnecessary queries to a backend. If the Supplier does
	// not have this feature, just return the input slice.
	FilterAvailableModules(modules []SupplierRequest) []SupplierRequest

	// TableForModule queries the Supplier for a given SymbolTable asynchronously.
	// Returns a channel on which the caller can receive the response.
	TableForModule(request SupplierRequest) <-chan SupplierResponse
}

// SupplierRequest is sent to a Supplier to get a SymbolTable, via a SupplierResponse.
type SupplierRequest struct {
	// The debug file name of a code module for which symbol information is requested.
	ModuleName string

	// The unique identifier for a version of the named module.
	Identifier string
}

// SupplierResponse is returned by a Supplier in response to a SupplierRequest.
type SupplierResponse struct {
	// Error is set if the SupplierRequest could not be serviced successuflly.
	Error error

	// The table found in response to the SupplierRequest.
	Table SymbolTable
}

// AnnotatedFrame is one stack frame that also has information about the module
// in which the instruction resides.
type AnnotatedFrame struct {
	Address uint64
	Module  SupplierRequest
}

// AnnotatedFrameService is an interface to a backend that can provide
// AnnotatedFrames for a given crash report identifier and crash key. The crash
// key is some field in the crash report that contains a stack that is to be
// returned.
type AnnotatedFrameService interface {
	// Returns a callstack of AnnotatedFrames for a given metadata key in
	// the specified crash report.
	GetAnnotatedFrames(reportID, key string) ([]AnnotatedFrame, error)
}

// ModuleInfoService is an interface that describes a way to look up module
// information for a specific product and version.
type ModuleInfoService interface {
	// Returns a list of modules a specific product and version.
	GetModulesForProduct(product, version string) ([]SupplierRequest, error)
}
